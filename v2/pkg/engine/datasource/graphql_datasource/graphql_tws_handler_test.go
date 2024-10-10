//go:build !race

package graphql_datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/flags"

	"github.com/stretchr/testify/assert"
	"nhooyr.io/websocket"
)

func TestWebsocketSubscriptionClient_GQLTWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
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
	).(*subscriptionClient)

	updater := &testSubscriptionUpdater{}
	go func() {
		rCtx := resolve.NewContext(ctx)
		rCtx.ExecutionOptions.SendHeartbeat = true
		err := client.Subscribe(rCtx, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()
	updater.AwaitUpdates(t, 10*time.Second, 4)
	assert.Equal(t, 4, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	assert.Equal(t, `{}`, updater.updates[3])

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestWebsocketSubscriptionClientPing_GQLTWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
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
	).(*subscriptionClient)

	updater := &testSubscriptionUpdater{}

	go func() {
		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()

	updater.AwaitUpdates(t, time.Second, 1)
	assert.Equal(t, 1, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestWebsocketSubscriptionClientError_GQLTWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
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
	)

	updater := &testSubscriptionUpdater{}

	go func() {
		err := client.Subscribe(resolve.NewContext(clientCtx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `wrongQuery {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()

	updater.AwaitUpdates(t, time.Second, 1)
	assert.Equal(t, 1, len(updater.updates))
	assert.Equal(t, `{"errors":[{"message":"Unexpected Name \"wrongQuery\"","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_PARSE_FAILED"}}]}`, updater.updates[0])

	clientCancel()
	updater.AwaitDone(t, time.Second)

	serverCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebSocketSubscriptionClientInitIncludePing_GQLTWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverDone := make(chan struct{})
	assertion := require.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
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
	).(*subscriptionClient)
	updater := &testSubscriptionUpdater{}

	go func() {
		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assertion.NoError(err)
	}()

	updater.AwaitUpdates(t, time.Second, 2)
	assertion.Equal(2, len(updater.updates))
	assertion.Equal(`{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assertion.Equal(`{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])

	clientCancel()
	assertion.Eventuallyf(func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestWebsocketSubscriptionClient_GQLTWS_Upstream_Dies(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
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
	).(*subscriptionClient)

	updater := &testSubscriptionUpdater{}

	go func() {
		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()

	updater.AwaitUpdates(t, time.Second, 1)
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])

	// Kill the upstream here. We should get an End-of-File error.
	assert.NoError(t, wrappedListener.underlyingConnection.Close())
	updater.AwaitUpdates(t, time.Second, 2)
	assert.Equal(t, `{"errors":[{"message":"failed to get reader: failed to read frame header: EOF"}]}`, updater.updates[1])

	clientCancel()
	serverCancel()
}
