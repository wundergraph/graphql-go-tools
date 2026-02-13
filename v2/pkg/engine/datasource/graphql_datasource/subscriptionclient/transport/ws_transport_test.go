package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

func TestWSTransport_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("dials and returns message channel", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read subscribe
			var msg map[string]any
			_ = wsjson.Read(ctx, conn, &msg)
			assert.Equal(t, "subscribe", msg["type"])

			// Send data
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      msg["id"],
				"type":    "next",
				"payload": map[string]any{"data": map[string]any{"value": 42}},
			})

			// Send complete
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":   msg["id"],
				"type": "complete",
			})
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "42")

		msg = receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("reuses connection for same endpoint", func(t *testing.T) {
		t.Parallel()

		var dialCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			dialCount.Add(1)

			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}

				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts)
		require.NoError(t, err)
		defer cancel1()

		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts)
		require.NoError(t, err)
		defer cancel2()

		// Both should receive messages
		receiveWithTimeout(t, ch1, time.Second)
		receiveWithTimeout(t, ch2, time.Second)

		// Only one connection should have been made
		assert.Equal(t, int32(1), dialCount.Load())
		assert.Equal(t, 1, tr.ConnCount())
	})

	t.Run("creates new connection for different headers", func(t *testing.T) {
		t.Parallel()

		var dialCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			dialCount.Add(1)

			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}

				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		headers1 := http.Header{"Authorization": []string{"Bearer token1"}}
		headers2 := http.Header{"Authorization": []string{"Bearer token2"}}

		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
			Headers:   headers1,
		})
		require.NoError(t, err)
		defer cancel1()

		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
			Headers:   headers2,
		})
		require.NoError(t, err)
		defer cancel2()

		receiveWithTimeout(t, ch1, time.Second)
		receiveWithTimeout(t, ch2, time.Second)

		// Two connections due to different headers
		assert.Equal(t, int32(2), dialCount.Load())
		assert.Equal(t, 2, tr.ConnCount())
	})

	t.Run("creates new connection for different init payload", func(t *testing.T) {
		t.Parallel()

		var dialCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			dialCount.Add(1)

			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}

				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:    server.URL,
			Transport:   common.TransportWS,
			InitPayload: map[string]any{"token": "abc"},
		})
		require.NoError(t, err)
		defer cancel1()

		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, common.Options{
			Endpoint:    server.URL,
			Transport:   common.TransportWS,
			InitPayload: map[string]any{"token": "xyz"},
		})
		require.NoError(t, err)
		defer cancel2()

		receiveWithTimeout(t, ch1, time.Second)
		receiveWithTimeout(t, ch2, time.Second)

		// Two connections due to different init payload
		assert.Equal(t, int32(2), dialCount.Load())
		assert.Equal(t, 2, tr.ConnCount())
	})

	t.Run("removes connection when all subscriptions closed", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		_, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts)
		require.NoError(t, err)

		_, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts)
		require.NoError(t, err)

		assert.Equal(t, 1, tr.ConnCount())

		cancel1()
		assert.Equal(t, 1, tr.ConnCount()) // still has one subscription

		cancel2()

		// Wait for onEmpty callback
		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("redials after connection closed", func(t *testing.T) {
		t.Parallel()

		var dialCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			dialCount.Add(1)

			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}

				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		// First subscription
		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts)
		require.NoError(t, err)
		receiveWithTimeout(t, ch1, time.Second)
		cancel1()

		// Wait for connection to be removed
		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)

		// Second subscription should redial
		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts)
		require.NoError(t, err)
		defer cancel2()
		receiveWithTimeout(t, ch2, time.Second)

		assert.Equal(t, int32(2), dialCount.Load())
	})
}

func TestWSTransport_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("context cancellation closes all connections", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
			}
		})

		ctx, cancel := context.WithCancel(context.Background())
		tr := transport.NewWSTransport(ctx, http.DefaultClient, nil)

		_, _, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)

		assert.Equal(t, 1, tr.ConnCount())

		cancel()

		// Wait for cleanup
		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("context cancellation notifies subscribers", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
			}
		})

		ctx, cancel := context.WithCancel(context.Background())
		tr := transport.NewWSTransport(ctx, http.DefaultClient, nil)

		ch, _, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)

		cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.Error(t, msg.Err)
		assert.True(t, msg.Done)
	})
}

func TestWSTransport_ConcurrentSubscribe(t *testing.T) {
	t.Parallel()

	t.Run("handles concurrent subscribes to same endpoint", func(t *testing.T) {
		t.Parallel()

		var dialCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			dialCount.Add(1)

			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}

				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { test }"}, opts)
				if err != nil {
					return
				}
				defer cancel()

				receiveWithTimeout(t, ch, time.Second)
			})
		}

		wg.Wait()

		// Should have only dialed once (or maybe twice due to race, but not 10 times)
		assert.LessOrEqual(t, dialCount.Load(), int32(2))
	})
}

func TestWSTransport_InitPayloadForwarding(t *testing.T) {
	t.Parallel()

	t.Run("forwards init payload to server with graphql-transport-ws protocol", func(t *testing.T) {
		t.Parallel()

		receivedPayload := make(chan map[string]any, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{"graphql-transport-ws"},
			})
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			// Read connection_init and capture payload
			var initMsg map[string]any
			if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
				return
			}
			if initMsg["type"] != "connection_init" {
				return
			}
			if payload, ok := initMsg["payload"].(map[string]any); ok {
				receivedPayload <- payload
			} else {
				receivedPayload <- nil
			}

			_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

			// Read subscribe and respond
			var subMsg map[string]any
			if err := wsjson.Read(ctx, conn, &subMsg); err != nil {
				return
			}
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      subMsg["id"],
				"type":    "next",
				"payload": map[string]any{"data": map[string]any{"value": 1}},
			})
		}))
		t.Cleanup(server.Close)

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		initPayload := map[string]any{
			"Authorization": "Bearer secret-token",
			"X-Custom":      "custom-value",
			"nested": map[string]any{
				"key": "nested-value",
			},
		}

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   initPayload,
		})
		require.NoError(t, err)
		defer cancel()

		// Verify payload was received by server
		select {
		case payload := <-receivedPayload:
			require.NotNil(t, payload, "server should receive init payload")
			assert.Equal(t, "Bearer secret-token", payload["Authorization"])
			assert.Equal(t, "custom-value", payload["X-Custom"])
			nested, ok := payload["nested"].(map[string]any)
			require.True(t, ok, "nested should be a map")
			assert.Equal(t, "nested-value", nested["key"])
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for init payload")
		}

		// Subscription should work
		msg := receiveWithTimeout(t, ch, time.Second)
		assert.NotNil(t, msg.Payload)
	})

	t.Run("forwards init payload to server with graphql-ws legacy protocol", func(t *testing.T) {
		t.Parallel()

		receivedPayload := make(chan map[string]any, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{"graphql-ws"},
			})
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			// Read connection_init and capture payload
			var initMsg map[string]any
			if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
				return
			}
			if initMsg["type"] != "connection_init" {
				return
			}
			if payload, ok := initMsg["payload"].(map[string]any); ok {
				receivedPayload <- payload
			} else {
				receivedPayload <- nil
			}

			_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

			// Read start and respond
			var startMsg map[string]any
			if err := wsjson.Read(ctx, conn, &startMsg); err != nil {
				return
			}
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      startMsg["id"],
				"type":    "data",
				"payload": map[string]any{"data": map[string]any{"value": 1}},
			})
		}))
		t.Cleanup(server.Close)

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		initPayload := map[string]any{
			"token":   "legacy-auth-token",
			"version": float64(2), // JSON numbers are float64
		}

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS, // Legacy protocol
			InitPayload:   initPayload,
		})
		require.NoError(t, err)
		defer cancel()

		// Verify payload was received by server
		select {
		case payload := <-receivedPayload:
			require.NotNil(t, payload, "server should receive init payload")
			assert.Equal(t, "legacy-auth-token", payload["token"])
			assert.Equal(t, float64(2), payload["version"])
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for init payload")
		}

		// Subscription should work
		msg := receiveWithTimeout(t, ch, time.Second)
		assert.NotNil(t, msg.Payload)
	})

	t.Run("sends empty payload when init payload is nil", func(t *testing.T) {
		t.Parallel()

		receivedPayload := make(chan map[string]any, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{"graphql-transport-ws"},
			})
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			// Read connection_init and capture payload
			var initMsg map[string]any
			if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
				return
			}
			if initMsg["type"] != "connection_init" {
				return
			}
			if payload, ok := initMsg["payload"].(map[string]any); ok {
				receivedPayload <- payload
			} else {
				receivedPayload <- nil
			}

			_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

			// Read subscribe and respond
			var subMsg map[string]any
			if err := wsjson.Read(ctx, conn, &subMsg); err != nil {
				return
			}
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      subMsg["id"],
				"type":    "next",
				"payload": map[string]any{"data": map[string]any{"value": 1}},
			})
		}))
		t.Cleanup(server.Close)

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   nil, // No init payload
		})
		require.NoError(t, err)
		defer cancel()

		// Server should receive nil/empty payload
		select {
		case payload := <-receivedPayload:
			assert.Nil(t, payload, "server should receive nil payload when not provided")
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for init message")
		}

		// Subscription should still work
		msg := receiveWithTimeout(t, ch, time.Second)
		assert.NotNil(t, msg.Payload)
	})

	t.Run("same endpoint with different init payloads uses separate connections", func(t *testing.T) {
		t.Parallel()

		var mu sync.Mutex
		receivedPayloads := make([]map[string]any, 0)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{"graphql-transport-ws"},
			})
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			// Read connection_init and capture payload
			var initMsg map[string]any
			if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
				return
			}
			if initMsg["type"] != "connection_init" {
				return
			}

			mu.Lock()
			if payload, ok := initMsg["payload"].(map[string]any); ok {
				receivedPayloads = append(receivedPayloads, payload)
			}
			mu.Unlock()

			_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

			// Handle subscriptions
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
				if msg["type"] == "subscribe" {
					_ = wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		}))
		t.Cleanup(server.Close)

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		// First subscription with user1 token
		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   map[string]any{"user": "user1"},
		})
		require.NoError(t, err)
		defer cancel1()

		receiveWithTimeout(t, ch1, time.Second)

		// Second subscription with user2 token - should create new connection
		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   map[string]any{"user": "user2"},
		})
		require.NoError(t, err)
		defer cancel2()

		receiveWithTimeout(t, ch2, time.Second)

		// Verify two separate connections were made with different payloads
		assert.Equal(t, 2, tr.ConnCount())

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, receivedPayloads, 2)

		users := make([]string, 0, 2)
		for _, p := range receivedPayloads {
			if user, ok := p["user"].(string); ok {
				users = append(users, user)
			}
		}
		assert.ElementsMatch(t, []string{"user1", "user2"}, users)
	})
}

func TestWSTransport_LegacyProtocol(t *testing.T) {
	t.Parallel()

	t.Run("connects to legacy graphql-ws server", func(t *testing.T) {
		t.Parallel()

		server := newLegacyGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read start message
			var msg map[string]any
			require.NoError(t, wsjson.Read(ctx, conn, &msg))
			assert.Equal(t, "start", msg["type"])

			// Send data
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      msg["id"],
				"type":    "data",
				"payload": map[string]any{"data": map[string]any{"value": 42}},
			})

			// Send complete
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":   msg["id"],
				"type": "complete",
			})
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS, // Request legacy protocol
		})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "42")

		msg = receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("handles keep-alive messages", func(t *testing.T) {
		t.Parallel()

		server := newLegacyGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read start message
			var msg map[string]any
			require.NoError(t, wsjson.Read(ctx, conn, &msg))

			// Send keep-alive
			_ = wsjson.Write(ctx, conn, map[string]string{"type": "ka"})

			// Send data
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      msg["id"],
				"type":    "data",
				"payload": map[string]any{"data": map[string]any{"value": 1}},
			})

			// Send complete
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":   msg["id"],
				"type": "complete",
			})
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS,
		})
		require.NoError(t, err)
		defer cancel()

		// Should receive data (keep-alive is handled internally)
		msg := receiveWithTimeout(t, ch, time.Second)
		assert.NotNil(t, msg.Payload)

		msg = receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("auto-negotiates to legacy when modern unavailable", func(t *testing.T) {
		t.Parallel()

		// Server only supports legacy protocol
		server := newLegacyGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			var msg map[string]any
			require.NoError(t, wsjson.Read(ctx, conn, &msg))
			assert.Equal(t, "start", msg["type"]) // Should use legacy message type

			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      msg["id"],
				"type":    "data",
				"payload": map[string]any{"data": map[string]any{"value": 99}},
			})
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":   msg["id"],
				"type": "complete",
			})
		})

		tr := transport.NewWSTransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolAuto, // Auto-negotiate
		})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "99")
	})
}

// Test helpers

func newGraphQLWSServer(t *testing.T, handler func(ctx context.Context, conn *websocket.Conn)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Handle connection_init
		var initMsg map[string]any
		if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
			return
		}
		if initMsg["type"] != "connection_init" {
			return
		}
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

		handler(ctx, conn)
	}))

	t.Cleanup(server.Close)
	return server
}

func newLegacyGraphQLWSServer(t *testing.T, handler func(ctx context.Context, conn *websocket.Conn)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-ws"}, // Legacy protocol only
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Handle connection_init
		var initMsg map[string]any
		if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
			return
		}
		if initMsg["type"] != "connection_init" {
			return
		}
		_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

		handler(ctx, conn)
	}))

	t.Cleanup(server.Close)
	return server
}
