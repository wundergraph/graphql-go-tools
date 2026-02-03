package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"go.uber.org/goleak"
)

func TestClient(t *testing.T) {
	t.Run("new creates client with transports", func(t *testing.T) {
		c := New(t.Context(), Config{})

		assert.NotNil(t, c.ws)
		assert.NotNil(t, c.sse)
	})

	t.Run("context cancellation is idempotent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		_ = New(ctx, Config{})
		cancel()
		cancel() // should not panic
	})

	t.Run("subscribe fails after context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := New(ctx, Config{})
		cancel()

		_, _, err := c.Subscribe(t.Context(), &Request{Query: "subscription { a }"}, Options{
			Endpoint: "ws://localhost/graphql",
		})

		assert.Equal(t, ErrClientClosed, err)
	})
}

func TestClient_ContextCancellation(t *testing.T) {
	// These tests verify that cancelling the client's context properly cleans up all goroutines

	t.Run("context cancellation cleans up", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreAnyFunction("net/http/httptest.(*Server).goServe.func1"))

		server := newTestWSServer(t)

		ctx, cancel := context.WithCancel(context.Background())

		c := New(ctx, Config{})

		ch, _, err := c.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)

		// subscription is working
		select {
		case msg := <-ch:
			require.NotNil(t, msg.Payload)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
		}

		cancel()
	})

	t.Run("context cancellation cleans up multiple connections", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreAnyFunction("net/http/httptest.(*Server).goServe.func1"))

		server := newTestWSServer(t)

		ctx, cancel := context.WithCancel(context.Background())

		c := New(ctx, Config{})

		// Start subscriptions with different headers (forces multiple connections)
		for i := range 3 {
			headers := http.Header{"X-Request-ID": []string{string(rune('A' + i))}}
			ch, _, err := c.Subscribe(context.Background(), &common.Request{
				Query: "subscription { test }",
			}, common.Options{
				Endpoint:  server.URL,
				Transport: common.TransportWS,
				Headers:   headers,
			})
			require.NoError(t, err)

			// Drain first message
			select {
			case <-ch:
			case <-time.After(time.Second):
				t.Fatal("timeout")
			}
		}

		// Should have 3 connections
		stats := c.Stats()
		require.Equal(t, 3, stats.WSConns)

		cancel()
	})
}

func TestClient_CancelSendsComplete(t *testing.T) {
	t.Run("cancel sends complete to server", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreAnyFunction("net/http/httptest.(*Server).goServe.func1"))

		completeReceived := make(chan string, 1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols: []string{"graphql-transport-ws"},
			})
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx := r.Context()

			// Handle connection_init
			var initMsg map[string]any
			if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
				return
			}
			if initMsg["type"] != "connection_init" {
				return
			}
			_ = wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

			// Read subscribe
			var subMsg map[string]any
			if err := wsjson.Read(ctx, conn, &subMsg); err != nil {
				return
			}
			if subMsg["type"] != "subscribe" {
				return
			}
			subID := subMsg["id"].(string)

			// Send a next message
			_ = wsjson.Write(ctx, conn, map[string]any{
				"id":      subID,
				"type":    "next",
				"payload": map[string]any{"data": map[string]any{"value": 1}},
			})

			// Wait for complete message from client
			var completeMsg map[string]any
			if err := wsjson.Read(ctx, conn, &completeMsg); err != nil {
				return
			}
			if completeMsg["type"] == "complete" {
				completeReceived <- completeMsg["id"].(string)
			}
		}))
		t.Cleanup(server.Close)

		c := New(t.Context(), Config{})

		ch, cancel, err := c.Subscribe(t.Context(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)

		// Wait for first message
		select {
		case msg := <-ch:
			require.NotNil(t, msg.Payload)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
		}

		// Cancel the subscription - this should send complete to server
		cancel()

		// Verify server received complete
		select {
		case id := <-completeReceived:
			assert.NotEmpty(t, id, "complete message should have subscription ID")
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for complete message on server")
		}
	})
}

// Test helper: creates an SSE server that sends periodic messages
func newTestSSEServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// Send initial message
		fmt.Fprintf(w, "event: next\ndata: {\"data\":{\"test\":\"value\"}}\n\n")
		flusher.Flush()

		// Send periodic messages until client disconnects
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprintf(w, "event: next\ndata: {\"data\":{\"test\":\"value\"}}\n\n")
				flusher.Flush()
			}
		}
	}))

	t.Cleanup(server.Close)
	return server
}

// Test helper: creates a WebSocket server that sends periodic messages
func newTestWSServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"graphql-transport-ws"},
		})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Handle connection_init
		var initMsg map[string]any
		if err := wsjson.Read(ctx, conn, &initMsg); err != nil {
			return
		}
		if initMsg["type"] != "connection_init" {
			return
		}
		wsjson.Write(ctx, conn, map[string]string{"type": "connection_ack"})

		// Read messages in background, cancel context when connection closes
		go func() {
			defer cancel()
			for {
				var msg map[string]any
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					return
				}
				// Handle subscribe by sending first message
				if msg["type"] == "subscribe" {
					wsjson.Write(ctx, conn, map[string]any{
						"id":      msg["id"],
						"type":    "next",
						"payload": map[string]any{"data": map[string]any{"value": 1}},
					})
				}
			}
		}()

		// Send periodic messages until context cancelled
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Ignore write errors (connection may be closed)
				_ = wsjson.Write(ctx, conn, map[string]any{
					"type":    "next",
					"payload": map[string]any{"data": map[string]any{"value": 1}},
				})
			}
		}
	}))

	t.Cleanup(server.Close)
	return server
}
