package graphql_datasource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

func logger() ll.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return ll.NewZapLogger(logger, ll.DebugLevel)
}

func TestWebsocketSubscriptionClient(t *testing.T) {
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

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
		close(serverDone)
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	first := <-next
	second := <-next
	third := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
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
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
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

	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	first := <-next
	second := <-next
	third := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	assert.Eventuallyf(t, func() bool {
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClientErrorArray(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomNam: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"error","id":"1","payload":[{"message":"error"},{"message":"error"}]}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
		_, _, err = conn.Read(r.Context())
		assert.NotNil(t, err)
		close(serverDone)
	}))
	defer server.Close()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	clientCtx, clientCancel := context.WithCancel(context.Background())
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	next := make(chan []byte)
	err := client.Subscribe(clientCtx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomNam: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	message := <-next
	assert.Equal(t, `{"errors":[{"message":"error"},{"message":"error"}]}`, string(message))
	clientCancel()
	_, ok := <-next
	assert.False(t, ok)
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebsocketSubscriptionClientErrorObject(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomNam: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"error","id":"1","payload":{"message":"error"}}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
		_, _, err = conn.Read(r.Context())
		assert.NotNil(t, err)
		close(serverDone)
	}))
	defer server.Close()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	clientCtx, clientCancel := context.WithCancel(context.Background())
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	next := make(chan []byte)
	err := client.Subscribe(clientCtx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomNam: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	message := <-next
	assert.Equal(t, `{"errors":[{"message":"error"}]}`, string(message))
	clientCancel()
	_, ok := <-next
	assert.False(t, ok)
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
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

	assertReceiveMessages := func(next chan []byte) {
		first := <-next
		second := <-next
		third := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
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
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	clientsDone := &sync.WaitGroup{}

	next := make(chan []byte)
	ctx, clientCancel := context.WithCancel(context.Background())
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	assertReceiveMessages(next)

	for i := 0; i < 3; i++ {
		clientsDone.Add(1)
		next := make(chan []byte)

		ctx, cancel := context.WithCancel(context.Background())

		err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
			URL: strings.Replace(server.URL, "http", "ws", -1),
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, next)
		assert.NoError(t, err)
		go func(next chan []byte, cancel func()) {
			assertReceiveMessages(next)
			cancel()
			clientsDone.Done()
		}(next, cancel)
	}

	clientCancel()

	serverDone.Wait()
	clientsDone.Wait()
	assert.Eventuallyf(t, func() bool {
		return connectedClients.Load() == 0
	}, time.Second, time.Millisecond, "clients not 0")
}

func TestWebsocketSubscriptionClientWithInitPayload(t *testing.T) {
	assertInitAck := func(ctx context.Context, conn *websocket.Conn) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init", "payload":{"Authorization":"Bearer XXX"}}`, string(data))
		err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
	}

	handshakeHappened := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		assertInitAck(r.Context(), conn)
		handshakeHappened = true
		<-conn.CloseRead(r.Context()).Done()
	}))
	defer server.Close()

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, clientCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	subscribeHeaders := http.Header{}
	subscribeHeaders.Add("Authorization", "Bearer XXX")
	next := make(chan []byte)
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	err := client.Subscribe(subCtx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		Header: subscribeHeaders,
	}, next)
	assert.NoError(t, err)

	assert.Len(t, client.handlers, 1, "handler not registered")
	assert.Eventuallyf(t, func() bool {
		return handshakeHappened
	}, time.Second, time.Millisecond, "handshake was not performed")
}

func TestConnectionInitMessage(t *testing.T) {
	for i, tc := range []struct {
		header          http.Header
		expectedMessage string
	}{
		{
			header:          http.Header{},
			expectedMessage: `{"type":"connection_init"}`,
		},
		{
			header:          nil,
			expectedMessage: `{"type":"connection_init"}`,
		},
		{
			header:          http.Header{"Foo": []string{"bar"}},
			expectedMessage: `{"type":"connection_init", "payload":{"Foo":"bar"}}`,
		},
		{
			header:          http.Header{"Foo": []string{"bar", "baz"}},
			expectedMessage: `{"type":"connection_init", "payload":{"Foo":"bar"}}`,
		},
		{
			header:          http.Header{"Foo": []string{""}},
			expectedMessage: `{"type":"connection_init", "payload":{"Foo":""}}`,
		},
		{
			header:          http.Header{"Foo": []string{}},
			expectedMessage: `{"type":"connection_init", "payload":{"Foo":""}}`,
		},
		{
			header:          http.Header{"Foo": nil},
			expectedMessage: `{"type":"connection_init", "payload":{"Foo":""}}`,
		},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			msg, err := connectionInitMessage(tc.header)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMessage, msg)
		})
	}
}
