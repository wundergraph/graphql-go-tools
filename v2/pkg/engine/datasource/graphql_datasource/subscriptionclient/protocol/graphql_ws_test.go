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
		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
			wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})
		})

		conn := dialGWS(t, server)

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

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			time.Sleep(500 * time.Millisecond)
		})

		conn := dialGWS(t, server)

		p := &protocol.GraphQLWS{AckTimeout: 50 * time.Millisecond}
		err := p.Init(t.Context(), conn, nil)

		require.ErrorIs(t, err, protocol.ErrAckTimeout)
	})

	t.Run("handles keep-alive before ack", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg1 map[string]any
			wsjson.Read(ctx, conn, &msg1) // connection_init

			// Send keep-alive before ack
			wsjson.Write(ctx, conn, map[string]string{"type": "ka"})

			// Then send ack
			wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Init(t.Context(), conn, nil)
		require.NoError(t, err)
	})

	t.Run("returns error on connection_error", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			wsjson.Read(ctx, conn, &msg)
			wsjson.Write(ctx, conn, map[string]any{
				"type":    "connection_error",
				"payload": map[string]any{"message": "auth failed"},
			})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Init(t.Context(), conn, nil)

		require.ErrorIs(t, err, protocol.ErrConnectionError)
		assert.Contains(t, err.Error(), "auth failed")
	})

	t.Run("returns error on unexpected message type", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			wsjson.Read(ctx, conn, &msg)
			wsjson.Write(ctx, conn, map[string]string{"type": "error"})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Init(t.Context(), conn, nil)

		assert.ErrorIs(t, err, protocol.ErrAckNotReceived)
	})
}

func TestGraphQLWSLegacy_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("sends start message with query and variables", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 1)
		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Subscribe(t.Context(), conn, "sub-1", &common.Request{
			Query:     "subscription { test }",
			Variables: map[string]any{"id": 123},
		})
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "start", msg["type"])
			assert.Equal(t, "sub-1", msg["id"])

			payload, _ := msg["payload"].(map[string]any)
			assert.Equal(t, "subscription { test }", payload["query"])

			vars, _ := payload["variables"].(map[string]any)
			assert.Equal(t, float64(123), vars["id"])
		})
	})
}

func TestGraphQLWSLegacy_Unsubscribe(t *testing.T) {
	t.Parallel()

	t.Run("sends stop message with subscription id", func(t *testing.T) {
		t.Parallel()

		received := make(chan map[string]any, 1)
		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err == nil {
				received <- msg
			}
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		err := p.Unsubscribe(t.Context(), conn, "sub-1")
		require.NoError(t, err)

		awaitMessage(t, time.Second, received, func(t *testing.T, msg map[string]any) {
			assert.Equal(t, "stop", msg["type"])
			assert.Equal(t, "sub-1", msg["id"])
		})
	})
}

func TestGraphQLWSLegacy_Read(t *testing.T) {
	t.Parallel()

	t.Run("decodes data message with payload", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "data",
				"payload": map[string]any{
					"data": map[string]any{"value": 42},
				},
			})
		})

		conn := dialGWS(t, server)

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

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "error",
				"payload": []map[string]any{
					{"message": "something went wrong"},
				},
			})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, protocol.MessageError, msg.Type)
		require.Error(t, msg.Err)
		assert.Contains(t, msg.Err.Error(), "something went wrong")
	})

	t.Run("decodes complete message", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"id":   "sub-1",
				"type": "complete",
			})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, "sub-1", msg.ID)
		assert.Equal(t, protocol.MessageComplete, msg.Type)
	})

	t.Run("decodes keep-alive message as ping", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]string{"type": "ka"})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, protocol.MessagePing, msg.Type)
	})

	t.Run("decodes connection_error message", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]any{
				"type":    "connection_error",
				"payload": map[string]any{"reason": "session expired"},
			})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		msg, err := p.Read(t.Context(), conn)

		require.NoError(t, err)
		assert.Equal(t, protocol.MessageError, msg.Type)
		require.Error(t, msg.Err)
		assert.Contains(t, msg.Err.Error(), "session expired")
	})

	t.Run("returns error for unknown message type", func(t *testing.T) {
		t.Parallel()

		server := newGWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
			wsjson.Write(ctx, conn, map[string]string{"type": "unknown"})
		})

		conn := dialGWS(t, server)

		p := protocol.NewGraphQLWS()
		_, err := p.Read(t.Context(), conn)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})
}

func TestGraphQLWSLegacy_PingPong(t *testing.T) {
	t.Parallel()

	t.Run("ping is a no-op for legacy protocol", func(t *testing.T) {
		t.Parallel()

		// Legacy protocol doesn't support client-initiated ping
		p := protocol.NewGraphQLWS()

		// This should not error, just be a no-op
		err := p.Ping(context.Background(), nil)
		require.NoError(t, err)
	})

	t.Run("pong is a no-op for legacy protocol", func(t *testing.T) {
		t.Parallel()

		// Legacy protocol doesn't support pong
		p := protocol.NewGraphQLWS()

		// This should not error, just be a no-op
		err := p.Pong(context.Background(), nil)
		require.NoError(t, err)
	})
}

func newGWSTestServer(t *testing.T, handler func(ctx context.Context, conn *websocket.Conn)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-ws"},
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

func dialGWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(t.Context(), server.URL, &websocket.DialOptions{
		Subprotocols: []string{"graphql-ws"},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close(websocket.StatusNormalClosure, "")
	})

	return conn
}
