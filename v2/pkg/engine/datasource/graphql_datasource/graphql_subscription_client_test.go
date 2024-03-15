package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

func logger() ll.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return ll.NewZapLogger(logger, ll.DebugLevel)
}

func TestGetConnectionInitMessageHelper(t *testing.T) {
	var callback OnWsConnectionInitCallback = func(ctx context.Context, url string, header http.Header) (json.RawMessage, error) {
		return json.RawMessage(`{"authorization":"secret"}`), nil
	}

	tests := []struct {
		name     string
		callback *OnWsConnectionInitCallback
		want     string
	}{
		{
			name:     "without payload",
			callback: nil,
			want:     `{"type":"connection_init"}`,
		},
		{
			name:     "with payload",
			callback: &callback,
			want:     `{"type":"connection_init","payload":{"authorization":"secret"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := SubscriptionClient{onWsConnectionInitCallback: tt.callback}
			got, err := client.getConnectionInitMessage(context.Background(), "", nil)
			require.NoError(t, err)
			require.NotEmpty(t, got)

			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestWebsocketSubscriptionClientDeDuplication(t *testing.T) {
	serverDone := &sync.WaitGroup{}
	connectedClients := atomic.NewInt64(0)

	assertSubscription := func(ctx context.Context, conn *websocket.Conn, subscriptionID int) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, fmt.Sprintf(`{"type":"start","id":"%d","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, subscriptionID), string(data))
	}

	assertSendMessages := func(ctx context.Context, conn *websocket.Conn, subscriptionID int) {
		err := conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"first"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"second"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"third"}}}}`, subscriptionID)))
		assert.NoError(t, err)
	}

	assertInitAck := func(ctx context.Context, conn *websocket.Conn) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
	}

	assertReceiveMessages := func(t *testing.T, updater *testSubscriptionUpdater) {
		t.Helper()
		updater.AwaitUpdates(t, time.Second, 3)
		assert.Equal(t, 3, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	}

	assertStop := func(ctx context.Context, conn *websocket.Conn, subscriptionID ...int) {
		var receivedIDs []int
		expectedSum := 0
		actualSum := 0
		for _, expected := range subscriptionID {
			expectedSum += expected
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			messageType, err := jsonparser.GetString(data, "type")
			assert.NoError(t, err)
			assert.Equal(t, "stop", messageType)
			idStr, err := jsonparser.GetString(data, "id")
			assert.NoError(t, err)
			id, err := strconv.Atoi(idStr)
			assert.NoError(t, err)
			receivedIDs = append(receivedIDs, id)
			actualSum += id
		}
		assert.Len(t, receivedIDs, 4)
		assert.Equal(t, expectedSum, actualSum)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverDone.Add(1)
		defer serverDone.Done()
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		connectedClients.Inc()
		defer connectedClients.Dec()

		assertInitAck(r.Context(), conn)

		assertSubscription(r.Context(), conn, 1)
		assertSendMessages(r.Context(), conn, 1)

		assertSubscription(r.Context(), conn, 2)
		assertSubscription(r.Context(), conn, 3)
		assertSubscription(r.Context(), conn, 4)

		assertSendMessages(r.Context(), conn, 2)
		assertSendMessages(r.Context(), conn, 3)
		assertSendMessages(r.Context(), conn, 4)

		assertStop(r.Context(), conn, 1, 2, 3, 4)
	}))
	defer server.Close()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	clientsDone := &sync.WaitGroup{}

	updater := &testSubscriptionUpdater{}

	ctx, clientCancel := context.WithCancel(context.Background())
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
	assert.NoError(t, err)
	assertReceiveMessages(t, updater)

	for i := 0; i < 3; i++ {
		clientsDone.Add(1)

		updater := &testSubscriptionUpdater{}

		ctx, cancel := context.WithCancel(context.Background())

		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
		go func(updater *testSubscriptionUpdater, cancel func()) {
			assertReceiveMessages(t, updater)
			cancel()
			clientsDone.Done()
		}(updater, cancel)
	}

	clientCancel()

	serverDone.Wait()
	clientsDone.Wait()
	assert.Eventuallyf(t, func() bool {
		return connectedClients.Load() == 0
	}, time.Second, time.Millisecond, "clients not 0")
}

func TestWebsocketSubscriptionClientImmediateClientCancel(t *testing.T) {
	serverInvocations := atomic.NewInt64(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverInvocations.Inc()
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())
	clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	updater := &testSubscriptionUpdater{}
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
	assert.Error(t, err)
	assert.Eventuallyf(t, func() bool {
		return serverInvocations.Load() == 0
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClientWithServerDisconnect(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		ctx := context.Background()
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
		assert.NoError(t, err)

		_, _, err = conn.Read(ctx)
		assert.Error(t, err)
		close(serverDone)
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	updater := &testSubscriptionUpdater{}
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
	assert.NoError(t, err)
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	assert.Eventuallyf(t, func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}
