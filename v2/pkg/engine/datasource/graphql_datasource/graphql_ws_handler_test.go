//go:build !race

package graphql_datasource

import (
	"context"
	"net"
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

func TestWebSocketSubscriptionClientInitIncludeKA_GQLWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

	serverDone := make(chan struct{})
	assertion := require.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assertion.NoError(err)

		// write "ka" every second
		go func() {
			for {
				err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ka"}`))
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
		assertion.Equal(`{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assertion.NoError(err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assertion.NoError(err)
		assertion.NoError(err)

		msgType, data, err = conn.Read(ctx)
		assertion.NoError(err)
		assertion.Equal(websocket.MessageText, msgType)
		assertion.Equal(`{"type":"stop","id":"1"}`, string(data))
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
	assertion.Equal(`{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assertion.Equal(`{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	clientCancel()
	assertion.Eventuallyf(func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestWebsocketSubscriptionClient_GQLWS(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

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

func TestWebsocketSubscriptionClientErrorArray(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

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
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	updater := &testSubscriptionUpdater{}
	go func() {
		err := client.Subscribe(resolve.NewContext(clientCtx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomNam: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()
	updater.AwaitUpdates(t, time.Second, 1)
	assert.Equal(t, `{"errors":[{"message":"error"},{"message":"error"}]}`, updater.updates[0])
	clientCancel()
	updater.AwaitDone(t, time.Second)
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebsocketSubscriptionClientErrorObject(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

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
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)
	updater := &testSubscriptionUpdater{}
	go func() {
		err := client.Subscribe(resolve.NewContext(clientCtx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomNam: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
	}()
	updater.AwaitUpdates(t, time.Second, 1)
	assert.Equal(t, 1, len(updater.updates))
	assert.Equal(t, `{"errors":[{"message":"error"}]}`, updater.updates[0])
	clientCancel()
	updater.AwaitDone(t, time.Second)
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebsocketSubscriptionClient_GQLWS_Upstream_Dies(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skipping test on windows")
	}

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
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
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

	// Start a new GQL subscription and exchange some messages.
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
	assert.Equal(t, 1, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])

	// Kill the upstream here. We should get an End-of-File error.
	assert.NoError(t, wrappedListener.underlyingConnection.Close())
	updater.AwaitUpdates(t, time.Second, 2)
	assert.Equal(t, `{"errors":[{"message":"failed to get reader: failed to read frame header: EOF"}]}`, updater.updates[1])

	serverCancel()
	clientCancel()
}

type listenerWrapper struct {
	listener             net.Listener
	underlyingConnection net.Conn
}

func (l *listenerWrapper) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	l.underlyingConnection = conn
	return l.underlyingConnection, nil
}

func (l *listenerWrapper) Close() error {
	return l.listener.Close()
}

func (l *listenerWrapper) Addr() net.Addr {
	return l.listener.Addr()
}

var _ net.Listener = (*listenerWrapper)(nil)
