package transport

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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, handler)
		require.NoError(t, err)
		defer cancel()

		msg := receive(t, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "42")

		msg = receive(t, time.Second)
		assert.Equal(t, common.MessageTypeComplete, msg.Type)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		handler1, receive1 := collectingHandler()
		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts, handler1)
		require.NoError(t, err)
		defer cancel1()

		handler2, receive2 := collectingHandler()
		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts, handler2)
		require.NoError(t, err)
		defer cancel2()

		// Both should receive messages
		receive1(t, time.Second)
		receive2(t, time.Second)

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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		headers1 := http.Header{"Authorization": []string{"Bearer token1"}}
		headers2 := http.Header{"Authorization": []string{"Bearer token2"}}

		handler1, receive1 := collectingHandler()
		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
			Headers:   headers1,
		}, handler1)
		require.NoError(t, err)
		defer cancel1()

		handler2, receive2 := collectingHandler()
		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
			Headers:   headers2,
		}, handler2)
		require.NoError(t, err)
		defer cancel2()

		receive1(t, time.Second)
		receive2(t, time.Second)

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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler1, receive1 := collectingHandler()
		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:    server.URL,
			Transport:   common.TransportWS,
			InitPayload: map[string]any{"token": "abc"},
		}, handler1)
		require.NoError(t, err)
		defer cancel1()

		handler2, receive2 := collectingHandler()
		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, common.Options{
			Endpoint:    server.URL,
			Transport:   common.TransportWS,
			InitPayload: map[string]any{"token": "xyz"},
		}, handler2)
		require.NoError(t, err)
		defer cancel2()

		receive1(t, time.Second)
		receive2(t, time.Second)

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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts, func(_ *common.Message) {})
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		// First subscription
		handler1, receive1 := collectingHandler()
		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts, handler1)
		require.NoError(t, err)
		receive1(t, time.Second)
		cancel1()

		// Wait for connection to be removed
		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)

		// Second subscription should redial
		handler2, receive2 := collectingHandler()
		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts, handler2)
		require.NoError(t, err)
		defer cancel2()
		receive2(t, time.Second)

		assert.Equal(t, int32(2), dialCount.Load())
	})
}

func TestWSTransport_SubscriberDrain(t *testing.T) {
	t.Parallel()

	t.Run("connection closes when last subscriber cancels", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
			}
		})

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		cancel, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, func(_ *common.Message) {})
		require.NoError(t, err)

		assert.Equal(t, 1, tr.ConnCount())

		cancel()

		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("connection stays open while subscribers remain", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
			}
		})

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		opts := common.Options{Endpoint: server.URL, Transport: common.TransportWS}

		cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts, func(_ *common.Message) {})
		require.NoError(t, err)

		assert.Equal(t, 1, tr.ConnCount())

		cancel1()
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, tr.ConnCount(), "connection should stay open with remaining subscriber")

		cancel2()

		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{
			IdleTimeout: 30 * time.Second,
		})

		opts := common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				handler, receive := collectingHandler()
				cancel, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { test }"}, opts, handler)
				if err != nil {
					t.Errorf("subscribe error: %v", err)
					return
				}
				defer cancel()

				receive(t, time.Second)
			})
		}

		wg.Wait()

		assert.Equal(t, int32(1), dialCount.Load())
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		initPayload := map[string]any{
			"Authorization": "Bearer secret-token",
			"X-Custom":      "custom-value",
			"nested": map[string]any{
				"key": "nested-value",
			},
		}

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   initPayload,
		}, handler)
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
		msg := receive(t, time.Second)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		initPayload := map[string]any{
			"token":   "legacy-auth-token",
			"version": float64(2), // JSON numbers are float64
		}

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS, // Legacy protocol
			InitPayload:   initPayload,
		}, handler)
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
		msg := receive(t, time.Second)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   nil, // No init payload
		}, handler)
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
		msg := receive(t, time.Second)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		// First subscription with user1 token
		handler1, receive1 := collectingHandler()
		cancel1, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   map[string]any{"user": "user1"},
		}, handler1)
		require.NoError(t, err)
		defer cancel1()

		receive1(t, time.Second)

		// Second subscription with user2 token - should create new connection
		handler2, receive2 := collectingHandler()
		cancel2, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLTransportWS,
			InitPayload:   map[string]any{"user": "user2"},
		}, handler2)
		require.NoError(t, err)
		defer cancel2()

		receive2(t, time.Second)

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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS, // Request legacy protocol
		}, handler)
		require.NoError(t, err)
		defer cancel()

		msg := receive(t, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "42")

		msg = receive(t, time.Second)
		assert.Equal(t, common.MessageTypeComplete, msg.Type)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolGraphQLWS,
		}, handler)
		require.NoError(t, err)
		defer cancel()

		// Should receive data (keep-alive is handled internally)
		msg := receive(t, time.Second)
		assert.NotNil(t, msg.Payload)

		msg = receive(t, time.Second)
		assert.Equal(t, common.MessageTypeComplete, msg.Type)
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

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		handler, receive := collectingHandler()
		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:      server.URL,
			Transport:     common.TransportWS,
			WSSubprotocol: common.SubprotocolAuto, // Auto-negotiate
		}, handler)
		require.NoError(t, err)
		defer cancel()

		msg := receive(t, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "99")
	})
}

func TestWSTransport_Heartbeat(t *testing.T) {
	t.Parallel()

	t.Run("sends pings and receives pongs", func(t *testing.T) {
		t.Parallel()

		var pingCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read subscribe
			var msg map[string]any
			_ = wsjson.Read(ctx, conn, &msg)

			// Keep connection alive, respond to pings with pongs
			for {
				var incoming map[string]any
				if err := wsjson.Read(ctx, conn, &incoming); err != nil {
					return
				}
				if incoming["type"] == "ping" {
					pingCount.Add(1)
					_ = wsjson.Write(ctx, conn, map[string]string{"type": "pong"})
				}
			}
		})

		tr := NewWSTransport(t.Context(), WSTransportOptions{PingInterval: 50 * time.Millisecond})

		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, func(_ *common.Message) {})
		require.NoError(t, err)
		defer cancel()

		// Wait for at least 2 pings to be sent
		assert.Eventually(t, func() bool {
			return pingCount.Load() >= 2
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("closes connection on pong timeout", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read subscribe
			var msg map[string]any
			_ = wsjson.Read(ctx, conn, &msg)

			// Read pings but never respond with pong
			for {
				var incoming map[string]any
				if err := wsjson.Read(ctx, conn, &incoming); err != nil {
					return
				}
			}
		})

		tr := NewWSTransport(t.Context(), WSTransportOptions{PingInterval: 100 * time.Millisecond, PingTimeout: 50 * time.Millisecond})

		handler, receive := collectingHandler()
		_, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, handler)
		require.NoError(t, err)

		// Connection should be closed due to pong timeout, subscriber gets notified
		msg := receive(t, time.Second)
		assert.Equal(t, common.MessageTypeConnectionError, msg.Type)
		assert.Error(t, msg.Err)

		assert.Eventually(t, func() bool {
			return tr.ConnCount() == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("does not kill connection when ping timeout is disabled", func(t *testing.T) {
		t.Parallel()

		var pingCount atomic.Int32
		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read subscribe
			var msg map[string]any
			_ = wsjson.Read(ctx, conn, &msg)

			// Read pings but never respond with pong
			for {
				var incoming map[string]any
				if err := wsjson.Read(ctx, conn, &incoming); err != nil {
					return
				}
				if incoming["type"] == "ping" {
					pingCount.Add(1)
				}
			}
		})

		// PingInterval set, PingTimeout left at zero (disabled)
		tr := NewWSTransport(t.Context(), WSTransportOptions{PingInterval: 50 * time.Millisecond})

		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, func(_ *common.Message) {})
		require.NoError(t, err)
		defer cancel()

		// Wait for several ping cycles without any pong responses
		assert.Eventually(t, func() bool {
			return pingCount.Load() >= 3
		}, time.Second, 10*time.Millisecond)

		// Connection must still be alive despite no pongs
		assert.Equal(t, 1, tr.ConnCount())
	})

	t.Run("keeps connection alive when pongs arrive", func(t *testing.T) {
		t.Parallel()

		server := newGraphQLWSServer(t, func(ctx context.Context, conn *websocket.Conn) {
			// Read subscribe
			var msg map[string]any
			_ = wsjson.Read(ctx, conn, &msg)

			// Respond to pings with pongs
			for {
				var incoming map[string]any
				if err := wsjson.Read(ctx, conn, &incoming); err != nil {
					return
				}
				if incoming["type"] == "ping" {
					_ = wsjson.Write(ctx, conn, map[string]string{"type": "pong"})
				}
			}
		})

		tr := NewWSTransport(t.Context(), WSTransportOptions{PingInterval: 50 * time.Millisecond, PingTimeout: 200 * time.Millisecond})

		cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		}, func(_ *common.Message) {})
		require.NoError(t, err)
		defer cancel()

		// Connection should remain alive after several ping cycles
		time.Sleep(250 * time.Millisecond)
		assert.Equal(t, 1, tr.ConnCount())
	})
}

func TestWSTransport_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("applies default read limit when omitted", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		assert.Equal(t, defaultReadLimit, tr.ReadLimit())
	})

	t.Run("applies default read limit for zero value", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{ReadLimit: 0})

		assert.Equal(t, defaultReadLimit, tr.ReadLimit())
	})

	t.Run("overrides read limit when provided", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{ReadLimit: 2 * 1024 * 1024})

		assert.Equal(t, int64(2*1024*1024), tr.ReadLimit())
	})

	t.Run("ignores negative read limit", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{ReadLimit: -1})

		assert.Equal(t, defaultReadLimit, tr.ReadLimit())
	})

	t.Run("applies default write timeout", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{})

		assert.Equal(t, defaultWriteTimeout, tr.WriteTimeout())
	})

	t.Run("overrides write timeout when provided", func(t *testing.T) {
		t.Parallel()

		tr := NewWSTransport(t.Context(), WSTransportOptions{WriteTimeout: 10 * time.Second})

		assert.Equal(t, 10*time.Second, tr.WriteTimeout())
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
