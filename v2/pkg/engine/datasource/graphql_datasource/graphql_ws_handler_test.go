package graphql_datasource

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/pvormste/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestWebSocketSubscriptionClientInitIncludeKA_GQLWS(t *testing.T) {
	t.Skip("FIXME")

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
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
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

func TestWebsocketSubscriptionClient_GQLWS(t *testing.T) {
	t.Skip("FIXME")

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
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
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

func TestWebsocketSubscriptionClientErrorArray(t *testing.T) {
	t.Skip("FIXME")

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
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(resolve.NewContext(clientCtx), GraphQLSubscriptionOptions{
		URL: server.URL,
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
	t.Skip("FIXME")

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
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(resolve.NewContext(clientCtx), GraphQLSubscriptionOptions{
		URL: server.URL,
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

func TestWebsocketSubscriptionClient_GQLWS_Upstream_Dies(t *testing.T) {
	t.Skip("FIXME")

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
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
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

	serverCancel()
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketConnectionReuse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()
	ctx := context.Background()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	t.Run("reuse connections when they have no forwarded headers in common", func(t *testing.T) {
		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithReadTimeout(time.Millisecond),
			WithLogger(logger()),
			WithWSSubProtocol(ProtocolGraphQLWS),
		)

		resolveCtx1 := resolve.NewContext(ctx)
		err := client.Subscribe(resolveCtx1, GraphQLSubscriptionOptions{
			URL: server.URL,
		}, make(chan []byte))
		assert.NoError(t, err)

		resolveCtx2 := resolve.NewContext(ctx)
		err = client.Subscribe(resolveCtx2, GraphQLSubscriptionOptions{
			URL: server.URL,
		}, make(chan []byte))
		assert.NoError(t, err)

		assert.Len(t, client.handlers, 1)
	})

	const (
		headerName  = "X-Test-Header"
		headerValue = "test"
	)

	forwardedHeaderNames := []string{headerName}

	connectionReuseCases := []struct {
		Name                                    string
		ForwardedClientHeaderNames              []string
		ForwardedClientHeaderRegularExpressions []*regexp.Regexp
	}{
		{
			Name:                       "by header name",
			ForwardedClientHeaderNames: forwardedHeaderNames,
		},
		{
			Name:                                    "by regular expression",
			ForwardedClientHeaderRegularExpressions: []*regexp.Regexp{regexp.MustCompile("^X-.*")},
		},
	}

	t.Run("reuse connections when the forwarded header has the same value", func(t *testing.T) {
		for _, c := range connectionReuseCases {
			c := c
			t.Run(c.Name, func(t *testing.T) {
				client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
					WithReadTimeout(time.Millisecond),
					WithLogger(logger()),
					WithWSSubProtocol(ProtocolGraphQLWS),
				)

				resolveCtx1 := resolve.NewContext(ctx)
				resolveCtx1.Request.Header = make(http.Header)
				resolveCtx1.Request.Header.Set(headerName, headerValue)
				err := client.Subscribe(resolveCtx1, GraphQLSubscriptionOptions{
					URL:                                     server.URL,
					ForwardedClientHeaderNames:              c.ForwardedClientHeaderNames,
					ForwardedClientHeaderRegularExpressions: c.ForwardedClientHeaderRegularExpressions,
				}, make(chan []byte))
				assert.NoError(t, err)

				resolveCtx2 := resolve.NewContext(ctx)
				resolveCtx2.Request.Header = make(http.Header)
				resolveCtx2.Request.Header.Set(headerName, headerValue)
				err = client.Subscribe(resolveCtx2, GraphQLSubscriptionOptions{
					URL:                                     server.URL,
					ForwardedClientHeaderNames:              c.ForwardedClientHeaderNames,
					ForwardedClientHeaderRegularExpressions: c.ForwardedClientHeaderRegularExpressions,
				}, make(chan []byte))
				assert.NoError(t, err)

				assert.Len(t, client.handlers, 1)
			})
		}
	})

	t.Run("avoid reusing connections when a forwarded header has different values", func(t *testing.T) {
		for _, c := range connectionReuseCases {
			c := c
			t.Run(c.Name, func(t *testing.T) {
				client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
					WithReadTimeout(time.Millisecond),
					WithLogger(logger()),
					WithWSSubProtocol(ProtocolGraphQLWS),
				)

				resolveCtx1 := resolve.NewContext(ctx)
				resolveCtx1.Request.Header = make(http.Header)
				resolveCtx1.Request.Header.Set(headerName, "1")
				err := client.Subscribe(resolveCtx1, GraphQLSubscriptionOptions{
					URL:                                     server.URL,
					ForwardedClientHeaderNames:              c.ForwardedClientHeaderNames,
					ForwardedClientHeaderRegularExpressions: c.ForwardedClientHeaderRegularExpressions,
				}, make(chan []byte))
				assert.NoError(t, err)

				resolveCtx2 := resolve.NewContext(ctx)
				resolveCtx2.Request.Header = make(http.Header)
				resolveCtx2.Request.Header.Set(headerName, "2")
				err = client.Subscribe(resolveCtx2, GraphQLSubscriptionOptions{
					URL:                                     server.URL,
					ForwardedClientHeaderNames:              c.ForwardedClientHeaderNames,
					ForwardedClientHeaderRegularExpressions: c.ForwardedClientHeaderRegularExpressions,
				}, make(chan []byte))
				assert.NoError(t, err)

				assert.Len(t, client.handlers, 2)
			})
		}
	})
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
