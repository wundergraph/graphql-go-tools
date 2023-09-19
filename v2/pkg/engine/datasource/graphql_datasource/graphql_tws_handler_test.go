package graphql_datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"nhooyr.io/websocket"
)

func TestWebsocketSubscriptionClient_GQLTWS(t *testing.T) {
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
		assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
		assert.NoError(t, err)

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
		close(serverDone)
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())

	serverCtx, serverCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLTWS),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
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
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClientPing_GQLTWS(t *testing.T) {
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
		assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ping"}`))
		assert.NoError(t, err)

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"pong"}`, string(data))

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
		close(serverDone)
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())

	serverCtx, serverCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLTWS),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)

	first := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClientError_GQLTWS(t *testing.T) {
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
		assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"wrongQuery {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"payload":[{"message":"Unexpected Name \"wrongQuery\"","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_PARSE_FAILED"}}],"id":"1","type":"error"}`))
		assert.NoError(t, err)

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1",type":"complete"}`))
		assert.NoError(t, err)

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	clientCtx, clientCancel := context.WithCancel(context.Background())
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLTWS),
	)

	next := make(chan []byte)
	err := client.Subscribe(clientCtx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `wrongQuery {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)

	message := <-next
	assert.Equal(t, `{"errors":[{"message":"Unexpected Name \"wrongQuery\"","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_PARSE_FAILED"}}]}`, string(message))

	clientCancel()
	_, ok := <-next
	assert.False(t, ok)

	serverCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebSocketSubscriptionClientInitIncludePing_GQLTWS(t *testing.T) {
	serverDone := make(chan struct{})
	assertion := require.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assertion.NoError(err)

		// write "ping" every second
		go func() {
			for {
				err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ping"}`))
				if err != nil {
					break
				}
				time.Sleep(time.Second)
			}
		}()

		ctx := context.Background()
		msgType, data, err := conn.Read(ctx)
		assertion.NoError(err)

		assertion.Equal(websocket.MessageText, msgType)
		assertion.Equal(`{"type":"connection_init"}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assertion.NoError(err)

		msgType, data, err = conn.Read(ctx)
		assertion.NoError(err)
		assertion.Equal(websocket.MessageText, msgType)
		assertion.Equal(`{"type":"pong"}`, string(data))

		msgType, data, err = conn.Read(ctx)
		assertion.NoError(err)
		assertion.Equal(websocket.MessageText, msgType)
		assertion.Equal(`{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assertion.NoError(err)

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assertion.NoError(err)

		msgType, data, err = conn.Read(ctx)
		assertion.NoError(err)
		assertion.Equal(websocket.MessageText, msgType)
		assertion.Equal(`{"id":"1","type":"complete"}`, string(data))
		close(serverDone)
	}))

	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())
	serverCtx, serverCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLTWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assertion.NoError(err)

	first := <-next
	second := <-next
	assertion.Equal(`{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assertion.Equal(`{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assertion.Eventuallyf(func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
	assertion.Eventuallyf(func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClient_GQLTWS_Upstream_Dies(t *testing.T) {
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)

		<-serverCtx.Done()
	}))

	// Wrap the listener to hijack the underlying TCP connection.
	// Hijacking via http.ResponseWriter doesn't work because the WebSocket
	// client already hijacks the connection before us.
	wrappedListener := &listenerWrapper{
		listener: server.Listener,
	}
	server.Listener = wrappedListener
	server.Start()

	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Second),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLTWS),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)

	first := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))

	// Kill the upstream here. We should get an End-of-File error.
	assert.NoError(t, wrappedListener.underlyingConnection.Close())
	errorMessage := <-next
	assert.Equal(t, `{"errors":[{"message":"failed to get reader: failed to read frame header: EOF"}]}`, string(errorMessage))

	clientCancel()
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}
