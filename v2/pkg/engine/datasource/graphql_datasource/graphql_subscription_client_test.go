package graphql_datasource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"go.uber.org/atomic"
	"go.uber.org/goleak"
	"go.uber.org/zap"
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
			client := subscriptionClient{onWsConnectionInitCallback: tt.callback}
			got, err := client.getConnectionInitMessage(context.Background(), "", nil)
			require.NoError(t, err)
			require.NotEmpty(t, got)

			assert.Equal(t, tt.want, string(got))
		})
	}
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
		assert.Error(t, err)
	}()
	assert.Eventuallyf(t, func() bool {
		return serverInvocations.Load() == 0
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
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

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))

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
}

// didnt configure subprotocol, but the subgraph return graphql-ws
func TestSubprotocolNegotiationWithGraphQLWS(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-ws"},
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
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

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
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

// didnt configure subprotocol, but the subgraph return graphql-transport-ws
func TestSubprotocolNegotiationWithGraphQLTransportWS(t *testing.T) {
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
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

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
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

// In this case the subgraph doesnt return the subprotocol and we didnt configure the subprotocol, so falls back to graphql-ws
func TestSubprotocolNegotiationWithNoSubprotocol(t *testing.T) {
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
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

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
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestSubprotocolNegotiationWithConfiguredGraphQLWS(t *testing.T) {
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
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

		WithLogger(logger()),
	).(*subscriptionClient)
	updater := &testSubscriptionUpdater{}
	go func() {
		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLWS,
		}, updater)
		assert.NoError(t, err)
	}()
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestSubprotocolNegotiationWithConfiguredGraphQLTransportWS(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "done")
		}()
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
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

		WithLogger(logger()),
	).(*subscriptionClient)
	updater := &testSubscriptionUpdater{}

	go func() {
		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)
	}()

	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestWebSocketClientLeaks(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreCurrent(), // ignore the test itself
	)
	serverDone := &sync.WaitGroup{}
	serverDone.Add(2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "done")
			serverDone.Done()
		}()
		ctx := context.Background()
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)

		time.Sleep(time.Second * 1)

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)

		time.Sleep(time.Second * 1)

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)

		time.Sleep(time.Second * 1)

		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
		assert.NoError(t, err)

		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
	}))
	defer server.Close()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

		WithLogger(logger()),
	).(*subscriptionClient)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			updater := &testSubscriptionUpdater{}
			err := client.SubscribeAsync(resolve.NewContext(ctx), uint64(i), GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			client.Unsubscribe(uint64(i))
			clientCancel()
			wg.Done()
		}(i)
	}
	wg.Wait()
	time.Sleep(time.Second)
	serverCancel()
	time.Sleep(time.Second)
	serverDone.Wait()
}

func TestAsyncSubscribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}
	t.Parallel()
	t.Run("subscribe async", func(t *testing.T) {
		t.Parallel()
		serverDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
				defer close(serverDone)
			}()
			ctx := context.Background()
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"type":"connection_init"}`, string(data))
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
			assert.NoError(t, err)

			time.Sleep(time.Second * 1)

			msgType, data, err = conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
			assert.NoError(t, err)

			time.Sleep(time.Second * 1)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
			assert.NoError(t, err)

			time.Sleep(time.Second * 1)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
			assert.NoError(t, err)

			msgType, data, err = conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*10, 3)
		assert.Equal(t, 3, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
		client.Unsubscribe(1)
		clientCancel()
		assert.Eventuallyf(t, func() bool {
			<-serverDone
			return true
		}, time.Second*5, time.Millisecond*10, "server did not close")
		serverCancel()
	})
	t.Run("server timeout", func(t *testing.T) {
		t.Parallel()
		serverDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
				close(serverDone)
			}()
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

			time.Sleep(time.Second * 2)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
			assert.NoError(t, err)

			msgType, data, err = conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*10, 3)
		assert.Equal(t, 3, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
		client.Unsubscribe(1)
		clientCancel()
		assert.Eventuallyf(t, func() bool {
			<-serverDone
			return true
		}, time.Second*5, time.Millisecond*10, "server did not close")
		serverCancel()
	})
	t.Run("server complete", func(t *testing.T) {
		t.Parallel()
		serverDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()
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

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"complete","id":"1"}`))
			assert.NoError(t, err)
			close(serverDone)
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*10, 1)
		assert.Equal(t, 1, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		client.Unsubscribe(1)
		clientCancel()
		assert.Eventuallyf(t, func() bool {
			<-serverDone
			return true
		}, time.Second, time.Millisecond*10, "server did not close")
		serverCancel()
	})
	t.Run("server ka", func(t *testing.T) {
		t.Parallel()
		serverDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()
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

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ka"}`))
			assert.NoError(t, err)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
			assert.NoError(t, err)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ka"}`))
			assert.NoError(t, err)

			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"complete","id":"1"}`))
			assert.NoError(t, err)
			close(serverDone)
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*10, 1)
		assert.Equal(t, 1, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		client.Unsubscribe(1)
		clientCancel()
		assert.Eventuallyf(t, func() bool {
			<-serverDone
			return true
		}, time.Second, time.Millisecond*10, "server did not close")
		serverCancel()
	})
	t.Run("long timeout", func(t *testing.T) {
		t.Parallel()
		serverDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()
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

			time.Sleep(time.Second * 2)

			close(serverDone)
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*10, 1)
		assert.Equal(t, 1, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		assert.Eventuallyf(t, func() bool {
			<-serverDone
			return true
		}, time.Second*5, time.Millisecond*10, "server did not close")
		time.Sleep(time.Second)
		assert.Equal(t, false, client.netPollState.hasConnections.Load())
	})
	t.Run("forever timeout", func(t *testing.T) {
		t.Parallel()
		globalCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()
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
			<-globalCtx.Done()
		}))
		defer server.Close()
		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
		).(*subscriptionClient)
		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		updater.AwaitUpdates(t, time.Second*3, 1)
		assert.Equal(t, 1, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		time.Sleep(time.Second * 2)
	})
	t.Run("graphql-ws", func(t *testing.T) {
		t.Parallel()
		t.Run("happy path", func(t *testing.T) {
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"data","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"data","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"data","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
				assert.NoError(t, err)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))

				ctx = conn.CloseRead(ctx)
				<-ctx.Done()
				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			client.Unsubscribe(1)

			clientCancel()

			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("connection error", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"connection_error"}`))
				assert.NoError(t, err)

				_ = conn.Close(websocket.StatusNormalClosure, "done")

				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*5, 1)
			assert.Equal(t, 1, len(updater.updates))
			assert.Equal(t, `{"errors":[{"message":"connection error"}]}`, updater.updates[0])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
			assert.Equal(t, false, client.netPollState.hasConnections.Load())
		})
		t.Run("error object", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"error","payload":{"message":"ws error"}}`))
				assert.NoError(t, err)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*5, 1)
			assert.Equal(t, 1, len(updater.updates))
			assert.Equal(t, `{"errors":[{"message":"ws error"}]}`, updater.updates[0])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("error array", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
					close(serverDone)
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"error","payload":[{"message":"ws error"}]}`))
				assert.NoError(t, err)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*5, 1)
			assert.Equal(t, 1, len(updater.updates))
			assert.Equal(t, `{"errors":[{"message":"ws error"}]}`, updater.updates[0])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
	})
	t.Run("graphql-transport-ws", func(t *testing.T) {
		t.Parallel()
		t.Run("happy path", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
				assert.NoError(t, err)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))

				ctx = conn.CloseRead(ctx)
				<-ctx.Done()
				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("happy path no netPoll", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

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
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
				WithNetPollConfiguration(NetPollConfiguration{
					Enable: false,
				}),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("happy path no netPoll two clients", func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
				assert.NoError(t, err)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"complete"}`, string(data))
			}))
			defer server.Close()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
				WithNetPollConfiguration(NetPollConfiguration{
					Enable: false,
				}),
			).(*subscriptionClient)
			wg := &sync.WaitGroup{}
			wg.Add(2)
			for i := 0; i < 2; i++ {
				go func(i int) {
					ctx, clientCancel := context.WithCancel(context.Background())
					defer clientCancel()
					updater := &testSubscriptionUpdater{}
					err := client.SubscribeAsync(resolve.NewContext(ctx), uint64(i), GraphQLSubscriptionOptions{
						URL: server.URL,
						Body: GraphQLBody{
							Query: `subscription {messageAdded(roomName: "room"){text}}`,
						},
						WsSubProtocol: ProtocolGraphQLTWS,
					}, updater)
					assert.NoError(t, err)

					updater.AwaitUpdates(t, time.Second*10, 3)
					assert.Equal(t, 3, len(updater.updates))
					assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
					assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
					assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
					clientCancel()
					wg.Done()
				}(i)
			}
			wg.Wait()
		})
		t.Run("ping", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
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
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("ka", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ka"}`))
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
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*10, 3)
			assert.Equal(t, 3, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
			assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second*5, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("error object", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"error","payload":{"message":"ws error"}}`))
				assert.NoError(t, err)

				_ = conn.Close(websocket.StatusNormalClosure, "done")

				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,

				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*5, 2)
			assert.Equal(t, 2, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"errors":[{"message":"ws error"}]}`, updater.updates[1])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("error array", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"error","payload":[{"message":"ws error"}]}`))
				assert.NoError(t, err)

				_ = conn.Close(websocket.StatusNormalClosure, "done")

				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)

			updater.AwaitUpdates(t, time.Second*5, 2)
			assert.Equal(t, 2, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			assert.Equal(t, `{"errors":[{"message":"ws error"}]}`, updater.updates[1])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("data error", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"data","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)
			updater.AwaitUpdates(t, time.Second*5, 1)
			assert.Equal(t, 1, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
		})
		t.Run("connection error", func(t *testing.T) {
			t.Parallel()
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				assert.NoError(t, err)
				defer func() {
					_ = conn.Close(websocket.StatusNormalClosure, "done")
				}()
				ctx := context.Background()
				msgType, data, err := conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"type":"connection_init"}`, string(data))
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
				assert.NoError(t, err)

				time.Sleep(time.Second * 1)

				msgType, data, err = conn.Read(ctx)
				assert.NoError(t, err)
				assert.Equal(t, websocket.MessageText, msgType)
				assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
				assert.NoError(t, err)

				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"connection_error"}`))
				assert.NoError(t, err)

				close(serverDone)
			}))
			defer server.Close()
			ctx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)
			updater := &testSubscriptionUpdater{}

			err := client.SubscribeAsync(resolve.NewContext(ctx), 1, GraphQLSubscriptionOptions{
				URL: server.URL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
				WsSubProtocol: ProtocolGraphQLTWS,
			}, updater)
			assert.NoError(t, err)
			updater.AwaitUpdates(t, time.Second*5, 1)
			assert.Equal(t, 1, len(updater.updates))
			assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
			client.Unsubscribe(1)
			clientCancel()
			assert.Eventuallyf(t, func() bool {
				<-serverDone
				return true
			}, time.Second, time.Millisecond*10, "server did not close")
			serverCancel()
		})
	})
}
