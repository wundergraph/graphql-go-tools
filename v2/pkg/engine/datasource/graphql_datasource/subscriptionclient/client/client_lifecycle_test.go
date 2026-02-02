package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/client"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

func TestClient_ContextCancellation(t *testing.T) {
	// These tests verify that cancelling the client's context properly cleans up all goroutines

	t.Run("context cancellation cleans up", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreAnyFunction("net/http/httptest.(*Server).goServe.func1"))

		server := newTestWSServer(t)

		ctx, cancel := context.WithCancel(context.Background())

		c := client.New(ctx, http.DefaultClient, http.DefaultClient)

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

		c := client.New(ctx, http.DefaultClient, http.DefaultClient)

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
