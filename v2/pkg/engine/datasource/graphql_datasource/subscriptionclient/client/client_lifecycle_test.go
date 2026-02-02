package client_test

import (
	"context"
	"fmt"
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

		c := client.New(ctx, client.Config{})

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

		c := client.New(ctx, client.Config{})

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

func TestClient_SubscriberContextCancellation(t *testing.T) {
	t.Run("SSE subscription survives when first subscriber context is cancelled", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreAnyFunction("net/http/httptest.(*Server).goServe.func1"))

		server := newTestSSEServer(t)

		c := client.New(context.Background(), client.Config{})

		// First subscriber with cancellable context
		subscriber1Ctx, cancelSubscriber1 := context.WithCancel(context.Background())

		ch1, cancel1, err := c.Subscribe(subscriber1Ctx, &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportSSE,
		})
		require.NoError(t, err)
		defer cancel1()

		// Wait for first message to confirm subscription is active
		select {
		case msg := <-ch1:
			require.NotNil(t, msg.Payload)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for first message on ch1")
		}

		// Second subscriber joins the same subscription (dedup)
		ch2, cancel2, err := c.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportSSE,
		})
		require.NoError(t, err)
		defer cancel2()

		// Verify dedup
		stats := c.Stats()
		require.Equal(t, 1, stats.Subscriptions)
		require.Equal(t, 2, stats.Listeners)

		// Cancel first subscriber's context (NOT their cancel function)
		cancelSubscriber1()
		time.Sleep(100 * time.Millisecond)

		// Second subscriber should still receive messages
		select {
		case msg, ok := <-ch2:
			require.True(t, ok, "channel should not be closed")
			require.Nil(t, msg.Err, "should not have error")
			require.NotNil(t, msg.Payload, "should have payload")
		case <-time.After(time.Second):
			t.Fatal("second subscriber should still receive messages")
		}
	})

	t.Run("deduped listener is removed when its context is cancelled", func(t *testing.T) {
		server := newTestWSServer(t)

		clientCtx, cancelClient := context.WithCancel(context.Background())
		defer cancelClient()

		c := client.New(clientCtx, client.Config{})

		// First subscriber
		ch1, cancel1, err := c.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)
		defer cancel1()

		// Wait for subscription to be active
		select {
		case <-ch1:
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}

		// Second subscriber with cancellable context
		subscriber2Ctx, cancelSubscriber2 := context.WithCancel(context.Background())

		_, cancel2, err := c.Subscribe(subscriber2Ctx, &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)
		defer cancel2()

		// Verify dedup
		stats := c.Stats()
		require.Equal(t, 1, stats.Subscriptions)
		require.Equal(t, 2, stats.Listeners)

		// Cancel second subscriber's context (NOT their cancel function)
		cancelSubscriber2()
		time.Sleep(100 * time.Millisecond)

		stats = c.Stats()
		require.Equal(t, 1, stats.Listeners)
	})

	t.Run("listener is removed when its context is cancelled", func(t *testing.T) {
		server := newTestWSServer(t)

		clientCtx, cancelClient := context.WithCancel(context.Background())
		defer cancelClient()

		c := client.New(clientCtx, client.Config{})

		// Subscriber with cancellable context
		subscriberCtx, cancelSubscriber := context.WithCancel(context.Background())

		ch, cancel, err := c.Subscribe(subscriberCtx, &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportWS,
		})
		require.NoError(t, err)
		defer cancel()

		// Wait for subscription to be active
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}

		stats := c.Stats()
		require.Equal(t, 1, stats.Subscriptions)
		require.Equal(t, 1, stats.Listeners)

		// Cancel subscriber's context (NOT the cancel function)
		cancelSubscriber()
		time.Sleep(100 * time.Millisecond)

		stats = c.Stats()
		require.Equal(t, 0, stats.Listeners, "listener should be removed when context is cancelled")
		require.Equal(t, 0, stats.Subscriptions, "subscription should be removed when last listener is gone")
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
