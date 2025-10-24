package graphql_datasource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/coder/websocket"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"
	"go.uber.org/zap"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
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
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}
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

func TestClientToSubgraphPingPong(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows as it's not reliable")
	}

	t.Run("client sends ping message after configured interval", func(t *testing.T) {
		t.Parallel()

		serverDone := make(chan struct{}) // to signal server done
		// buffered channels and non-blocking send to avoid double-close panics if events repeat
		pingReceived := make(chan struct{}, 1) // signaled when the server receives a ping
		payloadSend := make(chan struct{}, 1)  // signaled when the server sends a payload

		// Create test server that will handle the WebSocket connection
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Sec-WebSocket-Protocol", ProtocolGraphQLTWS)
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{ProtocolGraphQLTWS},
			})
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
				close(serverDone)
			}()

			ctx := context.Background()

			// Handle connection initialization
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"type":"connection_init"}`, string(data))
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
			assert.NoError(t, err)

			// Handle subscription start
			msgType, data, err = conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

			// Send initial data
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"initial data"}}}}`))
			assert.NoError(t, err)

			// Track what messages we've received
			receivedPing := false
			receivedComplete := false

			// Create a context with timeout for reading messages
			readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			// Process up to 5 messages or until we see both a ping and a complete
			for i := 0; i < 5; i++ {
				if receivedPing && receivedComplete {
					break
				}

				_, data, err = conn.Read(readCtx)
				if err != nil {
					// Connection closed or timeout
					t.Logf("Connection read ended: %v", err)
					break
				}

				messageStr := string(data)
				t.Logf("Received message: %s", messageStr)

				switch messageStr {
				case `{"type":"ping"}`:
					receivedPing = true
					select {
					case pingReceived <- struct{}{}:
					default:
					}
					err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"pong"}`))
					assert.NoError(t, err)
					err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"after ping-pong"}}}}`))
					assert.NoError(t, err)
					select {
					case payloadSend <- struct{}{}:
					default:
					}
				case `{"id":"1","type":"complete"}`:
					receivedComplete = true
				}
			}

			// Test is successful if we received a ping message
			if !receivedPing {
				t.Error("Did not receive ping message from client")
			}
		}))
		defer server.Close()

		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		// Create subscription client with a short ping interval for testing
		pingInterval := 400 * time.Millisecond
		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
			WithPingInterval(pingInterval),
			WithPingTimeout(1*time.Second),
			WithNetPollConfiguration(NetPollConfiguration{
				Enable:           true,
				TickInterval:     100 * time.Millisecond,
				BufferSize:       10,
				MaxEventWorkers:  2,
				WaitForNumEvents: 1,
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

		// Wait for ping to be received before unsubscribing
		select {
		case <-pingReceived:
			t.Log("Ping received successfully")
		case <-time.After(2 * time.Second):
			t.Log("Timed out waiting for ping, will unsubscribe anyway")
		}
		// don't unsubscribe immediately, give the server time to send a payload
		select {
		case <-payloadSend:
			t.Log("Payload sent successfully")
		case <-time.After(2 * time.Second):
			t.Log("Timed out waiting for sent payload, will unsubscribe anyway")
		}

		// Cleanup
		client.Unsubscribe(1)

		// Wait for server to finish
		select {
		case <-serverDone:
			// Server completed successfully
		case <-time.After(5 * time.Second):
			t.Fatal("Timed out waiting for server to complete")
		}

		clientCancel()
		serverCancel()
	})

	t.Run("client responds with pong when server sends ping", func(t *testing.T) {
		t.Parallel()

		pongReceived := make(chan struct{})
		serverDone := make(chan struct{})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Sec-WebSocket-Protocol", ProtocolGraphQLTWS)
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{ProtocolGraphQLTWS},
			})
			assert.NoError(t, err)
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()

			ctx := context.Background()

			// Handle connection initialization
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"type":"connection_init"}`, string(data))
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
			assert.NoError(t, err)

			// Handle subscription start
			msgType, data, err = conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
			assert.Equal(t, `{"id":"1","type":"subscribe","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))

			// Send initial data
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"initial data"}}}}`))
			assert.NoError(t, err)

			// Send ping message
			err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ping"}`))
			assert.NoError(t, err)

			// Wait for pong response from client
			msgType, data, err = conn.Read(ctx)
			if err != nil {
				t.Errorf("Error reading pong: %v", err)
				return
			}

			if string(data) == `{"type":"pong"}` {
				assert.Equal(t, websocket.MessageText, msgType)
				// Send another data message
				err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"1","type":"next","payload":{"data":{"messageAdded":{"text":"after ping-pong"}}}}`))
				assert.NoError(t, err)

				close(pongReceived)
			}

			// Wait for client to unsubscribe (complete message)
			readTimeout := time.NewTimer(3 * time.Second)
			defer readTimeout.Stop()

			readDone := make(chan struct{})
			go func() {
				msgType, data, _ = conn.Read(ctx)
				close(readDone)
			}()

			select {
			case <-readDone:
				// Successfully read client message
			case <-readTimeout.C:
				// Timeout is fine, we're just waiting for unsubscribe
			}

			close(serverDone)
		}))
		defer server.Close()

		ctx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()
		serverCtx, serverCancel := context.WithCancel(context.Background())
		defer serverCancel()

		client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
			WithLogger(logger()),
			WithReadTimeout(100*time.Millisecond),
			WithNetPollConfiguration(NetPollConfiguration{
				Enable:           true,
				TickInterval:     100 * time.Millisecond,
				BufferSize:       10,
				MaxEventWorkers:  2,
				WaitForNumEvents: 1,
			}),
		).(*subscriptionClient)

		updater := &testSubscriptionUpdater{}

		err := client.SubscribeAsync(resolve.NewContext(ctx), 2, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
			WsSubProtocol: ProtocolGraphQLTWS,
		}, updater)
		assert.NoError(t, err)

		select {
		case <-pongReceived:
			t.Log("Server received pong successfully")
		case <-time.After(2 * time.Second):
			t.Log("Timed out waiting for pong in server, will unsubscribe anyway")
		}

		// Verify we receive at least the initial data
		updater.mux.Lock()
		updatesCount := len(updater.updates)
		firstUpdate := ""
		if updatesCount > 0 {
			firstUpdate = updater.updates[0]
		}
		updater.mux.Unlock()

		assert.GreaterOrEqual(t, updatesCount, 1)
		assert.Equal(t, `{"data":{"messageAdded":{"text":"initial data"}}}`, firstUpdate)

		// Cleanup
		client.Unsubscribe(2)
		t.Log("client unsubscribed")

		// Wait for server to finish
		select {
		case <-serverDone:
			// Server completed successfully
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for server to complete")
		}

		clientCancel()
		serverCancel()
	})
}

func TestClientClosesConnectionOnPingTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows as it's not reliable")
	}

	t.Parallel()

	serverDone := make(chan struct{})
	pingReceived := make(chan struct{}, 1) // Buffer 1 in case ping arrives slightly late

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Sec-WebSocket-Protocol", ProtocolGraphQLTWS)
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{ProtocolGraphQLTWS},
		})
		assert.NoError(t, err)
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "server done")
			close(serverDone)
		}()

		ctx := context.Background()

		// Handle connection initialization
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)

		// Handle subscription start
		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		// Use regexp because client might generate different IDs
		assert.Regexp(t, `{"id":".*","type":"subscribe","payload":{.*}}`, string(data))
		// More specific check for query
		payloadQuery, err := jsonparser.GetString(data, "payload", "query")
		if assert.NoError(t, err) {
			assert.Equal(t, `subscription {messageAdded(roomName: "room"){text}}`, payloadQuery)
		}

		// Send initial data
		subID, err := jsonparser.GetString(data, "id") // Get the actual ID used by the client
		if !assert.NoError(t, err) {
			return
		}
		initialDataMsg := fmt.Sprintf(`{"id":"%s","type":"next","payload":{"data":{"messageAdded":{"text":"initial data"}}}}`, subID)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(initialDataMsg))
		assert.NoError(t, err)

		// Wait for ping, but DO NOT send pong
		readCtx, cancelRead := context.WithTimeout(ctx, 5*time.Second) // Timeout for reading messages
		defer cancelRead()

		hasReceivedPing := false
		for !hasReceivedPing {
			_, data, err = conn.Read(readCtx)
			if err != nil {
				t.Logf("Server read error (expected after client closes): %v", err)
				// Expecting an error here eventually as the client should close the connection
				assert.Error(t, err, "Server should encounter read error when client closes connection due to ping timeout")
				// Signal that the server is done (connection closed)
				close(serverDone)
				return // Exit handler goroutine
			}

			messageStr := string(data)
			t.Logf("Server received: %s", messageStr)
			if messageStr == `{"type":"ping"}` {
				t.Log("Server received ping, NOT sending pong.")
				hasReceivedPing = true
				select {
				case pingReceived <- struct{}{}:
				default: // Avoid blocking if channel is full
				}
			} else if strings.Contains(messageStr, `"type":"complete"`) {
				// Client might send complete before closing if test runs fast
				t.Log("Server received complete from client.")
			} else {
				t.Logf("Server received unexpected message type: %s", messageStr)
			}
		}

		// Keep reading until the connection is closed by the client
		for {
			// Use a timeout context to make sure we don't hang indefinitely
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, data, err = conn.Read(readCtx)
			cancel()

			if err != nil {
				t.Logf("Server read error after ping (expected): %v", err)
				assert.Error(t, err, "Server should encounter read error after client closes connection")
				return // Exit handler goroutine
			}

			// Log any messages received before connection close
			t.Logf("Server still receiving messages: %s", string(data))
		}
	}))
	defer server.Close()

	ctx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithLogger(logger()),
		WithPingInterval(500*time.Millisecond),
		WithPingTimeout(100*time.Millisecond),
		// Need netpoll enabled for ping/pong handling
		WithNetPollConfiguration(NetPollConfiguration{
			Enable:           true,
			TickInterval:     100 * time.Millisecond,
			BufferSize:       10,
			MaxEventWorkers:  2,
			WaitForNumEvents: 1,
		}),
	).(*subscriptionClient)

	updater := &testSubscriptionUpdater{}

	// Use a unique ID for async subscription
	subscriptionID := uint64(42)

	err := client.SubscribeAsync(resolve.NewContext(ctx), subscriptionID, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		WsSubProtocol: ProtocolGraphQLTWS,
	}, updater)
	assert.NoError(t, err)

	// Wait for initial data
	updater.AwaitUpdates(t, 3*time.Second, 1)
	updater.mux.Lock()
	updatesCount := len(updater.updates)
	firstUpdate := ""
	if updatesCount > 0 {
		firstUpdate = updater.updates[0]
	}
	updater.mux.Unlock()

	require.Equal(t, 1, updatesCount, "Client should receive initial data")
	assert.Equal(t, `{"data":{"messageAdded":{"text":"initial data"}}}`, firstUpdate)

	// Wait for the server to confirm it received a ping
	select {
	case <-pingReceived:
		t.Log("Test confirmed server received ping.")
	case <-time.After(3 * time.Second): // Should receive ping within ~pingInterval + read time
		t.Fatal("Timed out waiting for server to receive ping")
	}

	// Wait for server to signal it's done (connection closed by client)
	select {
	case <-serverDone:
		t.Log("Server confirmed connection closed.")
		// Success: server detected connection closure
	case <-time.After(5 * time.Second): // Should happen within ~2*pingInterval + processing time
		t.Fatal("Timed out waiting for server to detect connection closure")
	}

	// Explicitly unsubscribe just in case, although it should be closed already
	client.Unsubscribe(subscriptionID)
	clientCancel() // Cancel client context
	serverCancel() // Cancel server context (though serverDone should ensure it exited)
}

func TestWebSocketUpgradeFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		statusCode    int
		headers       map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:          "HTTP 400 Bad Request",
			statusCode:    http.StatusBadRequest,
			headers:       map[string]string{"Content-Type": "text/plain"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 401 Unauthorized",
			statusCode:    http.StatusUnauthorized,
			headers:       map[string]string{"WWW-Authenticate": "Bearer"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 403 Forbidden",
			statusCode:    http.StatusForbidden,
			headers:       map[string]string{"Content-Type": "application/json"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 404 Not Found",
			statusCode:    http.StatusNotFound,
			headers:       map[string]string{"Content-Type": "text/html"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 500 Internal Server Error",
			statusCode:    http.StatusInternalServerError,
			headers:       map[string]string{"Content-Type": "application/json"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 502 Bad Gateway",
			statusCode:    http.StatusBadGateway,
			headers:       map[string]string{"Content-Type": "text/html"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 503 Service Unavailable",
			statusCode:    http.StatusServiceUnavailable,
			headers:       map[string]string{"Retry-After": "60"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
		{
			name:          "HTTP 200 OK (wrong status for WebSocket)",
			statusCode:    http.StatusOK,
			headers:       map[string]string{"Content-Type": "application/json"},
			expectError:   true,
			errorContains: "failed to upgrade connection",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				w.WriteHeader(tc.statusCode)
				fmt.Fprintf(w, `{"error": "WebSocket upgrade failed", "status": %d}`, tc.statusCode)
			}))
			defer server.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)

			wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

			updater := &testSubscriptionUpdater{}
			err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
				URL: wsURL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
			}, updater)

			if tc.expectError {
				require.ErrorContains(t, err, tc.errorContains)

				// Verify the error is of the correct type
				var upgradeErr *UpgradeRequestError
				require.ErrorAs(t, err, &upgradeErr)
				require.Equal(t, tc.statusCode, upgradeErr.StatusCode)
				require.Equal(t, server.URL, upgradeErr.URL)
			} else {
				assert.NoError(t, err, "Expected no error for status code %d", tc.statusCode)
			}
		})
	}
}

func TestInvalidWebSocketAcceptKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		acceptKeyHandler func(challengeKey string) string
		expectError      bool
		errorContains    string
	}{
		{
			name: "Missing Sec-WebSocket-Accept header",
			acceptKeyHandler: func(challengeKey string) string {
				return "" // Don't set the header
			},
			expectError:   true,
			errorContains: "invalid Sec-WebSocket-Accept",
		},
		{
			name: "Malformed base64 Sec-WebSocket-Accept",
			acceptKeyHandler: func(challengeKey string) string {
				return "not-valid-base64!!!"
			},
			expectError:   true,
			errorContains: "invalid Sec-WebSocket-Accept",
		},
		{
			name: "Correct length but wrong content",
			acceptKeyHandler: func(challengeKey string) string {
				// 20 bytes (not the SHA-1 of challengeKey+GUID)
				return base64.StdEncoding.EncodeToString([]byte("12345678901234567890"))
			},
			expectError:   true,
			errorContains: "invalid Sec-WebSocket-Accept",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var receivedChallengeKey string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedChallengeKey = r.Header.Get("Sec-WebSocket-Key")
				require.NotEmpty(t, receivedChallengeKey, "Challenge key should be present in request")

				w.Header().Set("Upgrade", "websocket")
				w.Header().Set("Connection", "Upgrade")
				w.Header().Set("Sec-WebSocket-Version", "13")

				acceptKey := tc.acceptKeyHandler(receivedChallengeKey)
				if acceptKey != "" {
					w.Header().Set("Sec-WebSocket-Accept", acceptKey)
				}
				// If acceptKey is empty, we don't set the header (simulating missing header)

				w.WriteHeader(http.StatusSwitchingProtocols)

				// Close the connection immediately to prevent hanging
				// This simulates a server that sends 101 but then closes
				if hijacker, ok := w.(http.Hijacker); ok {
					conn, _, err := hijacker.Hijack()
					if err == nil {
						conn.Close()
					}
				}
			}))
			defer server.Close()

			// Create subscription client
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()

			client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
				WithLogger(logger()),
			).(*subscriptionClient)

			wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

			updater := &testSubscriptionUpdater{}
			err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
				URL: wsURL,
				Body: GraphQLBody{
					Query: `subscription {messageAdded(roomName: "room"){text}}`,
				},
			}, updater)

			require.Error(t, err)
			require.ErrorContains(t, err, tc.errorContains)
			require.NotEmpty(t, receivedChallengeKey)
		})
	}
}
