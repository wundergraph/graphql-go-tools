package websocket

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/subscription"
)

func TestGraphQLTransportWSMessageReader_Read(t *testing.T) {
	t.Run("should read a minimal message", func(t *testing.T) {
		data := []byte(`{ "type": "connection_init" }`)
		expectedMessage := &GraphQLTransportWSMessage{
			Type: "connection_init",
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})

	t.Run("should message with json payload", func(t *testing.T) {
		data := []byte(`{ "id": "1", "type": "connection_init", "payload": { "Authorization": "Bearer ey123" } }`)
		expectedMessage := &GraphQLTransportWSMessage{
			Id:      "1",
			Type:    "connection_init",
			Payload: []byte(`{ "Authorization": "Bearer ey123" }`),
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})

	t.Run("should read and deserialize subscribe message", func(t *testing.T) {
		data := []byte(`{ 
  "id": "1", 
  "type": "subscribe", 
  "payload": { 
    "operationName": "MyQuery", 
    "query": "query MyQuery($name: String) { hello(name: $name) }", 
    "variables": { "name": "Udo" },
    "extensions": { "Authorization": "Bearer ey123" }
  } 
}`)
		expectedMessage := &GraphQLTransportWSMessage{
			Id:   "1",
			Type: "subscribe",
			Payload: []byte(`{ 
    "operationName": "MyQuery", 
    "query": "query MyQuery($name: String) { hello(name: $name) }", 
    "variables": { "name": "Udo" },
    "extensions": { "Authorization": "Bearer ey123" }
  }`),
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)

		expectedPayload := &GraphQLTransportWSMessageSubscribePayload{
			OperationName: "MyQuery",
			Query:         "query MyQuery($name: String) { hello(name: $name) }",
			Variables:     []byte(`{ "name": "Udo" }`),
			Extensions:    []byte(`{ "Authorization": "Bearer ey123" }`),
		}
		actualPayload, err := reader.DeserializeSubscribePayload(message)
		assert.NoError(t, err)
		assert.Equal(t, expectedPayload, actualPayload)
	})
}

func TestGraphQLTransportWSMessageWriter_WriteConnectionAck(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteConnectionAck()
		assert.Error(t, err)
	})
	t.Run("should successfully write ack message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		err := writer.WriteConnectionAck()
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WritePing(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WritePing(nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write ping message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"ping"}`)
		err := writer.WritePing(nil)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should successfully write ping message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"ping","payload":{"connected_since":"10min"}}`)
		err := writer.WritePing([]byte(`{"connected_since":"10min"}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WritePong(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WritePong(nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write pong message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"pong"}`)
		err := writer.WritePong(nil)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should successfully write pong message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"pong","payload":{"connected_since":"10min"}}`)
		err := writer.WritePong([]byte(`{"connected_since":"10min"}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WriteNext(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteNext("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write next message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"next","payload":{"data":{"hello":"world"}}}`)
		err := writer.WriteNext("1", []byte(`{"data":{"hello":"world"}}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WriteError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteError("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write error message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
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

func TestGraphQLTransportWSMessageWriter_WriteComplete(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			Client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteComplete("1")
		assert.Error(t, err)
	})
	t.Run("should successfully write complete message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
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

func TestGraphQLTransportWSEventHandler_Emit(t *testing.T) {
	t.Run("should write on completed", func(t *testing.T) {
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		eventHandler.Emit(subscription.EventTypeOnSubscriptionCompleted, "1", nil, nil)
		expectedMessage := []byte(`{"id":"1","type":"complete"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on data", func(t *testing.T) {
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		eventHandler.Emit(subscription.EventTypeOnSubscriptionData, "1", []byte(`{ "data": { "hello": "world" } }`), nil)
		expectedMessage := []byte(`{"id":"1","type":"next","payload":{"data":{"hello":"world"}}}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write on non-subscription execution result", func(t *testing.T) {
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		go func() {
			eventHandler.Emit(subscription.EventTypeOnNonSubscriptionExecutionResult, "1", []byte(`{ "data": { "hello": "world" } }`), nil)
		}()

		assert.Eventually(t, func() bool {
			expectedDataMessage := []byte(`{"id":"1","type":"next","payload":{"data":{"hello":"world"}}}`)
			actualDataMessage := testClient.readMessageToClient()
			assert.Equal(t, expectedDataMessage, actualDataMessage)
			expectedCompleteMessage := []byte(`{"id":"1","type":"complete"}`)
			actualCompleteMessage := testClient.readMessageToClient()
			assert.Equal(t, expectedCompleteMessage, actualCompleteMessage)
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})
	t.Run("should write on error", func(t *testing.T) {
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		eventHandler.Emit(subscription.EventTypeOnError, "1", nil, errors.New("error occurred"))
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"error occurred"}]}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should execute the OnConnectionOpened event function", func(t *testing.T) {
		counter := 0
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		eventHandler.OnConnectionOpened = func() {
			counter++
		}
		eventHandler.Emit(subscription.EventTypeOnConnectionOpened, "", nil, nil)
		assert.Equal(t, counter, 1)
	})
	t.Run("should disconnect on duplicated subscriber id", func(t *testing.T) {
		testClient := NewTestClient(false)
		eventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		eventHandler.Emit(subscription.EventTypeOnDuplicatedSubscriberID, "1", nil, errors.New("subscriber already exists"))
		assert.False(t, testClient.IsConnected())
	})
}

func TestGraphQLTransportWSWriteEventHandler_HandleWriteEvent(t *testing.T) {
	t.Run("should write connection_ack", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypeConnectionAck, "", nil, nil)
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write ping", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypePing, "", nil, nil)
		expectedMessage := []byte(`{"type":"ping"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should write pong", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypePong, "", nil, nil)
		expectedMessage := []byte(`{"type":"pong"}`)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should close connection on invalid type", func(t *testing.T) {
		testClient := NewTestClient(false)
		writeEventHandler := NewTestGraphQLTransportWSEventHandler(testClient)
		writeEventHandler.HandleWriteEvent(GraphQLTransportWSMessageType("invalid"), "", nil, nil)
		assert.False(t, writeEventHandler.Writer.Client.IsConnected())
	})
}

func TestProtocolGraphQLTransportWSHandler_Handle(t *testing.T) {
	t.Run("should close connection when an unexpected message type is used", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"something"}`))
		assert.NoError(t, err)
		assert.False(t, testClient.IsConnected())
	})

	t.Run("for connection_init", func(t *testing.T) {
		t.Run("should time out if no connection_init message is sent", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("this test fails on Windows due to different timings than unix, consider fixing it at some point")
			}
			testClient := NewTestClient(false)
			protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)
			protocol.connectionInitTimeOutDuration = 2 * time.Millisecond
			protocol.eventHandler.OnConnectionOpened = protocol.startConnectionInitTimer

			protocol.eventHandler.Emit(subscription.EventTypeOnConnectionOpened, "", nil, nil)
			time.Sleep(10 * time.Millisecond)
			assert.True(t, protocol.connectionInitTimerStarted)
			assert.False(t, protocol.eventHandler.Writer.Client.IsConnected())
		})

		t.Run("should close connection after multiple connection_init messages", func(t *testing.T) {
			testClient := NewTestClient(false)
			protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)
			protocol.connectionInitTimeOutDuration = 50 * time.Millisecond
			protocol.eventHandler.OnConnectionOpened = protocol.startConnectionInitTimer

			ctrl := gomock.NewController(t)
			mockEngine := NewMockEngine(ctrl)

			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			protocol.eventHandler.Emit(subscription.EventTypeOnConnectionOpened, "", nil, nil)
			assert.Eventually(t, func() bool {
				expectedAckMessage := []byte(`{"type":"connection_ack"}`)
				time.Sleep(5 * time.Millisecond)
				err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_init"}`))
				assert.NoError(t, err)
				assert.Equal(t, expectedAckMessage, testClient.readMessageToClient())
				time.Sleep(1 * time.Millisecond)
				err = protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_init"}`))
				assert.NoError(t, err)
				assert.False(t, protocol.eventHandler.Writer.Client.IsConnected())
				return true
			}, 1*time.Second, 2*time.Millisecond)

		})

		t.Run("should not time out if connection_init message is sent before time out", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("this test fails on Windows due to different timings than unix, consider fixing it at some point")
			}
			testClient := NewTestClient(false)
			protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)
			protocol.heartbeatInterval = 4 * time.Millisecond
			protocol.connectionInitTimeOutDuration = 25 * time.Millisecond
			protocol.eventHandler.OnConnectionOpened = protocol.startConnectionInitTimer

			ctrl := gomock.NewController(t)
			mockEngine := NewMockEngine(ctrl)

			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			protocol.eventHandler.Emit(subscription.EventTypeOnConnectionOpened, "", nil, nil)
			assert.Eventually(t, func() bool {
				expectedAckMessage := []byte(`{"type":"connection_ack"}`)
				expectedHeartbeatMessage := []byte(`{"type":"pong","payload":{"type":"heartbeat"}}`)
				time.Sleep(1 * time.Millisecond)
				err := protocol.Handle(ctx, mockEngine, []byte(`{"type":"connection_init"}`))
				assert.NoError(t, err)
				assert.Equal(t, expectedAckMessage, testClient.readMessageToClient())
				time.Sleep(6 * time.Millisecond)
				assert.Equal(t, expectedHeartbeatMessage, testClient.readMessageToClient())
				time.Sleep(50 * time.Millisecond)
				assert.True(t, protocol.eventHandler.Writer.Client.IsConnected())
				assert.True(t, protocol.connectionInitTimerStarted)
				assert.Nil(t, protocol.connectionInitTimeOutCancel)
				return true
			}, 1*time.Second, 2*time.Millisecond)

		})
	})

	t.Run("should return pong on ping", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		assert.Eventually(t, func() bool {
			inputMessage := []byte(`{"type":"ping","payload":{"status":"ok"}}`)
			expectedMessage := []byte(`{"type":"pong","payload":{"status":"ok"}}`)
			err := protocol.Handle(ctx, mockEngine, inputMessage)
			assert.NoError(t, err)
			assert.Equal(t, expectedMessage, testClient.readMessageToClient())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("should handle subscribe", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		operation := []byte(`{"operationName":"Hello","query":"query Hello { hello }"}`)
		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)
		mockEngine.EXPECT().StartOperation(gomock.Eq(ctx), gomock.Eq("2"), gomock.Eq(operation), gomock.Eq(&protocol.eventHandler))

		assert.Eventually(t, func() bool {
			initMessage := []byte(`{"id":"1","type":"connection_init"}`)
			err := protocol.Handle(ctx, mockEngine, initMessage)
			assert.NoError(t, err)
			subscribeMessage := []byte(`{"id":"2","type":"subscribe","payload":` + string(operation) + `}`)
			err2 := protocol.Handle(ctx, mockEngine, subscribeMessage)
			assert.NoError(t, err2)
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("should handle complete", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)
		mockEngine.EXPECT().StopSubscription(gomock.Eq("1"), gomock.Eq(&protocol.eventHandler))

		assert.Eventually(t, func() bool {
			inputMessage := []byte(`{"id":"1","type":"complete"}`)
			err := protocol.Handle(ctx, mockEngine, inputMessage)
			assert.NoError(t, err)
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("should allow pong messages from client", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		assert.Eventually(t, func() bool {
			inputMessage := []byte(`{"type":"pong"}`)
			err := protocol.Handle(ctx, mockEngine, inputMessage)
			assert.NoError(t, err)
			assert.True(t, testClient.IsConnected())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("should not panic on broken input", func(t *testing.T) {
		testClient := NewTestClient(false)
		protocol := NewTestProtocolGraphQLTransportWSHandler(testClient)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		ctrl := gomock.NewController(t)
		mockEngine := NewMockEngine(ctrl)

		assert.Eventually(t, func() bool {
			inputMessage := []byte(`{"type":"connection_init","payload":{something}}`)
			err := protocol.Handle(ctx, mockEngine, inputMessage)
			assert.NoError(t, err)
			assert.False(t, testClient.IsConnected())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})
}

func NewTestGraphQLTransportWSEventHandler(testClient subscription.TransportClient) GraphQLTransportWSEventHandler {
	return GraphQLTransportWSEventHandler{
		logger: abstractlogger.Noop{},
		Writer: GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			mu:     &sync.Mutex{},
			Client: testClient,
		},
	}
}

func NewTestProtocolGraphQLTransportWSHandler(testClient subscription.TransportClient) *ProtocolGraphQLTransportWSHandler {
	return &ProtocolGraphQLTransportWSHandler{
		logger: abstractlogger.Noop{},
		reader: GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		},
		eventHandler:                  NewTestGraphQLTransportWSEventHandler(testClient),
		heartbeatInterval:             30,
		connectionInitTimeOutDuration: 10 * time.Second,
	}
}
