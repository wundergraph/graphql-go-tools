package protocol_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
)

func TestGraphQLWS_Init(t *testing.T) {
	t.Parallel()

	t.Run("sends connection_init and receives connection_ack", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 1)
		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
			wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()

		err := p.Init(t.Context(), conn, map[string]any{"secret": "token"})
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "connection_init", msg["type"])
			payload, _ := msg["payload"].(map[string]any)
			assert.Equal(t, "token", payload["secret"])
		})
	})

	t.Run("returns error when ack times out", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			time.Sleep(500 * time.Millisecond)
		})

		conn := dial(t, server)

		p := &protocol.GraphQLWS{AckTimeout: 50 * time.Millisecond}
		err := p.Init(t.Context(), conn, nil)

		require.ErrorIs(t, err, protocol.ErrAckTimeout)
	})

	t.Run("handles ping before ack", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 2)
		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg1 map[string]any
			wsjson.Read(ctx, conn, &msg1)
			received <- msg1

			wsjson.Write(ctx, conn, map[string]string{"type": "ping"})

			var msg2 map[string]any
			wsjson.Read(ctx, conn, &msg2)
			received <- msg2

			wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Init(t.Context(), conn, nil)
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "connection_init", msg["type"])
		})

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "pong", msg["type"])
		})
	})

	t.Run("returns error on unexpected message type", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			wsjson.Read(ctx, conn, &msg)
			wsjson.Write(ctx, conn, map[string]string{"type": "error"})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Init(t.Context(), conn, nil)

		assert.ErrorIs(t, err, protocol.ErrAckNotReceived)
	})
}

func TestGraphQLWS_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("sends subscribe message with query and variables", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 1)
		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Subscribe(t.Context(), conn, "sub-1", &common.Request{
			Query:     "subscription { test }",
			Variables: map[string]any{"id": 123},
		})
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "subscribe", msg["type"])
			assert.Equal(t, "sub-1", msg["id"])

			payload, _ := msg["payload"].(map[string]any)
			assert.Equal(t, "subscription { test }", payload["query"])

			vars, _ := payload["variables"].(map[string]any)
			assert.Equal(t, float64(123), vars["id"])
		})
	})
}

func TestGraphQLWS_Unsubscribe(t *testing.T) {
	t.Parallel()

	t.Run("sends complete message with subscription id", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 1)
		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Unsubscribe(t.Context(), conn, "sub-1")
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "complete", msg["type"])
			assert.Equal(t, "sub-1", msg["id"])
		})
	})
}

func TestGraphQLWS_Read(t *testing.T) {
	t.Parallel()

	t.Run("decodes next message with data payload", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "next",
				"payload": map[string]any{
					"data": map[string]any{"value": 42},
				},
			})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, "sub-1", msg.ID)
		assert.Equal(t, protocol.MessageData, msg.Type)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "42")
	})

	t.Run("decodes error message with graphql errors", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "error",
				"payload": []map[string]any{
					{"message": "something went wrong"},
				},
			})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, protocol.MessageError, msg.Type)
		require.Error(t, msg.Err)
		assert.Contains(t, msg.Err.Error(), "something went wrong")
	})

	t.Run("decodes complete message", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "complete",
			})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, "sub-1", msg.ID)
		assert.Equal(t, protocol.MessageComplete, msg.Type)
	})

	t.Run("decodes ping message", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]string{"type": "ping"})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, protocol.MessagePing, msg.Type)
	})

	t.Run("returns error for unknown message type", func(t *testing.T) {
		t.Parallel()

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]string{"type": "unknown"})
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		_, err := p.Read(t.Context(), conn)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})
}

func TestGraphQLWS_PingPong(t *testing.T) {
	t.Parallel()

	t.Run("sends ping message", func(t *testing.T) {
		received := make(chan map[string]any, 1)

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Ping(t.Context(), conn)
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "ping", msg["type"])
		})
	})

	t.Run("sends pong message", func(t *testing.T) {
		received := make(chan map[string]any, 1)

		server := newTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dial(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Pong(t.Context(), conn)
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "pong", msg["type"])
		})
	})
}

func newTestServer(t *testing.T, handler func(ctx context.Context, conn *websocket.Conn)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		handler(ctx, conn)
	}))

	t.Cleanup(server.Close)

	return server
}

func dial(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(t.Context(), server.URL, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close(websocket.StatusNormalClosure, "")
	})

	return conn
}

func awaitMessage[A any](t *testing.T, timeout time.Duration, ch <-chan A, f func(*testing.T, A), msgAndArgs ...any) {
	t.Helper()

	select {
	case msg := <-ch:
		f(t, msg)
	case <-time.After(timeout):
		t.Fatal("timed out waiting for message")
	}
}
