package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

func TestSSETransport_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("sends POST request and receives messages", func(t *testing.T) {
		t.Parallel()

		var receivedBody map[string]any
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify POST method
			assert.Equal(t, http.MethodPost, r.Method)

			// Verify headers
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
			assert.Equal(t, "no-cache", r.Header.Get("Cache-Control"))

			// Read and verify body
			body, _ := io.ReadAll(r.Body)
			assert.NoError(t, json.Unmarshal(body, &receivedBody))

			// Send SSE response
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)

			fmt.Fprintf(w, "event: next\ndata: {\"data\": {\"value\": 42}}\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query:         "subscription { test }",
			Variables:     []byte(`{"id": 123}`),
			OperationName: "TestSub",
		}, common.Options{
			Endpoint:  server.URL,
			Transport: common.TransportSSE,
		})
		require.NoError(t, err)
		defer cancel()

		// Verify request body
		assert.Equal(t, "subscription { test }", receivedBody["query"])
		assert.Equal(t, float64(123), receivedBody["variables"].(map[string]any)["id"])
		assert.Equal(t, "TestSub", receivedBody["operationName"])

		// Receive data message
		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "42")

		// Receive complete message
		msg = receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("passes custom headers", func(t *testing.T) {
		t.Parallel()

		var receivedAuth string
		var receivedCustom string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			receivedCustom = r.Header.Get("X-Custom-Header")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		headers := http.Header{
			"Authorization":   []string{"Bearer token123"},
			"X-Custom-Header": []string{"custom-value"},
		}

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint: server.URL,
			Headers:  headers,
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, "Bearer token123", receivedAuth)
		assert.Equal(t, "custom-value", receivedCustom)
	})

	t.Run("handles next event with data", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {\"user\": {\"name\": \"Alice\"}}}\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { user { name } }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "Alice")
		assert.False(t, msg.Done)
	})

	t.Run("handles error event", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			fmt.Fprintf(w, "event: error\ndata: [{\"message\": \"Something went wrong\"}]\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Errors), "Something went wrong")
	})

	t.Run("handles complete event", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
		assert.Nil(t, msg.Err)
		assert.Nil(t, msg.Payload)
	})

	t.Run("handles multi-line data", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			// Multi-line data per SSE spec
			fmt.Fprintf(w, "event: next\n")
			fmt.Fprintf(w, "data: {\"data\": {\n")
			fmt.Fprintf(w, "data:   \"value\": 42\n")
			fmt.Fprintf(w, "data: }}\n")
			fmt.Fprintf(w, "\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		// The multi-line data is joined with newlines
		assert.Contains(t, string(msg.Payload.Data), "42")
	})

	t.Run("ignores SSE comments", func(t *testing.T) {
		t.Parallel()

		var messageCount atomic.Int32
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)

			// Send some keep-alive comments
			fmt.Fprintf(w, ": keep-alive\n")
			fmt.Fprintf(w, ": another comment\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: next\ndata: {\"data\": {\"value\": 1}}\n\n")
			flusher.Flush()

			fmt.Fprintf(w, ": more keep-alive\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		// Should only receive 2 messages (next + complete), not comments
		for msg := range ch {
			messageCount.Add(1)
			if msg.Done {
				break
			}
		}

		assert.Equal(t, int32(2), messageCount.Load())
	})

	t.Run("cancel closes connection", func(t *testing.T) {
		t.Parallel()

		serverClosed := make(chan struct{})
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {}}\n\n")
			flusher.Flush()

			// Wait for client to disconnect
			<-r.Context().Done()
			close(serverClosed)
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)

		// Receive first message
		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, 1, tr.ConnCount())

		// Cancel should close the connection
		cancel()

		select {
		case <-serverClosed:
			// Good, server detected disconnect
		case <-time.After(time.Second):
			t.Fatal("server did not detect disconnect")
		}

		assert.Equal(t, 0, tr.ConnCount())
	})

	t.Run("context cancellation stops subscription", func(t *testing.T) {
		t.Parallel()

		serverClosed := make(chan struct{})
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {}}\n\n")
			flusher.Flush()

			<-r.Context().Done()
			close(serverClosed)
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ctx, ctxCancel := context.WithCancel(context.Background())

		ch, cancel, err := tr.Subscribe(ctx, &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		// Cancel context
		ctxCancel()

		select {
		case <-serverClosed:
		case <-time.After(time.Second):
			t.Fatal("server did not detect context cancellation")
		}
	})

	t.Run("handles non-200 response", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		_, _, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})

	t.Run("handles non-200 with body", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		_, _, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("creates separate connection per subscription", func(t *testing.T) {
		t.Parallel()

		var reqCount atomic.Int32
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			reqCount.Add(1)

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {}}\n\n")
			flusher.Flush()

			// Keep connection open
			<-r.Context().Done()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		opts := common.Options{Endpoint: server.URL}

		ch1, cancel1, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts)
		require.NoError(t, err)

		ch2, cancel2, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts)
		require.NoError(t, err)

		receiveWithTimeout(t, ch1, time.Second)
		receiveWithTimeout(t, ch2, time.Second)

		// SSE creates separate HTTP requests (no multiplexing)
		assert.Equal(t, int32(2), reqCount.Load())
		assert.Equal(t, 2, tr.ConnCount())

		cancel1()
		cancel2()
	})

	t.Run("handles server closing stream", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {\"value\": 1}}\n\n")
			flusher.Flush()

			// Server closes without sending complete
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.NotNil(t, msg.Payload)

		// Channel should close when server closes stream
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel should be closed")
		case <-time.After(time.Second):
			t.Fatal("channel should have been closed")
		}
	})

	t.Run("handles data without event type", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			// Some servers send data without explicit event type
			fmt.Fprintf(w, "data: {\"data\": {\"value\": 99}}\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "99")
	})
}

func TestSSETransport_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("context cancellation closes all connections", func(t *testing.T) {
		t.Parallel()

		var closedCount atomic.Int32
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {}}\n\n")
			flusher.Flush()

			<-r.Context().Done()
			closedCount.Add(1)
		})

		ctx, cancel := context.WithCancel(context.Background())
		tr := transport.NewSSETransport(ctx, http.DefaultClient, nil)

		opts := common.Options{Endpoint: server.URL}

		ch1, _, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { a }"}, opts)
		require.NoError(t, err)

		ch2, _, err := tr.Subscribe(context.Background(), &common.Request{Query: "subscription { b }"}, opts)
		require.NoError(t, err)

		receiveWithTimeout(t, ch1, time.Second)
		receiveWithTimeout(t, ch2, time.Second)

		assert.Equal(t, 2, tr.ConnCount())

		cancel()

		assert.Eventually(t, func() bool {
			return closedCount.Load() == 2
		}, time.Second, 10*time.Millisecond)

		assert.Equal(t, 0, tr.ConnCount())
	})
}

func TestSSETransport_CustomClient(t *testing.T) {
	t.Parallel()

	t.Run("uses custom http client", func(t *testing.T) {
		t.Parallel()

		var customHeaderReceived string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			customHeaderReceived = r.Header.Get("X-Custom-Client")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		// Custom client with transport that adds a header
		customClient := &http.Client{
			Transport: &headerTransport{
				base: http.DefaultTransport,
				headers: http.Header{
					"X-Custom-Client": []string{"test-client"},
				},
			},
		}

		tr := transport.NewSSETransport(t.Context(), customClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, "test-client", customHeaderReceived)
	})
}

// Test helpers

func newSSEServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

	return server
}

// headerTransport is a custom RoundTripper that adds headers to requests
type headerTransport struct {
	base    http.RoundTripper
	headers http.Header
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	maps.Copy(req.Header, t.headers)
	return t.base.RoundTrip(req)
}

func TestSSETransport_ContentTypeValidation(t *testing.T) {
	t.Parallel()

	t.Run("accepts text/event-stream with charset", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})
		require.NoError(t, err)
		defer cancel()

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("rejects non-SSE content type", func(t *testing.T) {
		t.Parallel()

		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"error": "not sse"}`))
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		_, _, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{Endpoint: server.URL})

		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "content-type") || strings.Contains(err.Error(), "Content-Type"))
	})
}

func TestSSETransport_GETMethod(t *testing.T) {
	t.Parallel()

	t.Run("sends GET request with query parameters", func(t *testing.T) {
		t.Parallel()

		var receivedMethod string
		var receivedQuery string
		var receivedVariables string
		var receivedOperationName string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedQuery = r.URL.Query().Get("query")
			receivedVariables = r.URL.Query().Get("variables")
			receivedOperationName = r.URL.Query().Get("operationName")

			// Verify no body for GET
			body, _ := io.ReadAll(r.Body)
			assert.Empty(t, body)

			// Verify headers
			assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
			assert.Equal(t, "no-cache", r.Header.Get("Cache-Control"))
			assert.Empty(t, r.Header.Get("Content-Type")) // No content-type for GET

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher := w.(http.Flusher)
			fmt.Fprintf(w, "event: next\ndata: {\"data\": {\"value\": 42}}\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: complete\ndata:\n\n")
			flusher.Flush()
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query:         "subscription { test }",
			Variables:     []byte(`{"id": 123}`),
			OperationName: "TestSub",
		}, common.Options{
			Endpoint:  server.URL,
			SSEMethod: common.SSEMethodGET,
		})
		require.NoError(t, err)
		defer cancel()

		// Verify GET method and query params
		assert.Equal(t, http.MethodGet, receivedMethod)
		assert.Equal(t, "subscription { test }", receivedQuery)
		assert.Equal(t, `{"id":123}`, receivedVariables)
		assert.Equal(t, "TestSub", receivedOperationName)

		// Receive data message
		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "42")

		// Receive complete message
		msg = receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)
	})

	t.Run("GET preserves existing query parameters", func(t *testing.T) {
		t.Parallel()

		var receivedToken string
		var receivedQuery string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedToken = r.URL.Query().Get("token")
			receivedQuery = r.URL.Query().Get("query")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL + "?token=abc123",
			SSEMethod: common.SSEMethodGET,
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, "abc123", receivedToken)
		assert.Equal(t, "subscription { test }", receivedQuery)
	})

	t.Run("GET passes custom headers", func(t *testing.T) {
		t.Parallel()

		var receivedAuth string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		headers := http.Header{
			"Authorization": []string{"Bearer token123"},
		}

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			SSEMethod: common.SSEMethodGET,
			Headers:   headers,
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, "Bearer token123", receivedAuth)
	})

	t.Run("GET omits empty variables and operationName", func(t *testing.T) {
		t.Parallel()

		var hasVariables bool
		var hasOperationName bool
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			hasVariables = r.URL.Query().Has("variables")
			hasOperationName = r.URL.Query().Has("operationName")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
			// No variables or operationName
		}, common.Options{
			Endpoint:  server.URL,
			SSEMethod: common.SSEMethodGET,
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.False(t, hasVariables, "variables should not be in query params")
		assert.False(t, hasOperationName, "operationName should not be in query params")
	})
}

func TestSSETransport_MethodDefault(t *testing.T) {
	t.Parallel()

	t.Run("defaults to POST when SSEMethod is auto", func(t *testing.T) {
		t.Parallel()

		var receivedMethod string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			SSEMethod: common.SSEMethodAuto, // or just omit it
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, http.MethodPost, receivedMethod)
	})

	t.Run("explicit POST method works", func(t *testing.T) {
		t.Parallel()

		var receivedMethod string
		server := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: complete\ndata:\n\n")
		})

		tr := transport.NewSSETransport(t.Context(), http.DefaultClient, nil)

		ch, cancel, err := tr.Subscribe(context.Background(), &common.Request{
			Query: "subscription { test }",
		}, common.Options{
			Endpoint:  server.URL,
			SSEMethod: common.SSEMethodPOST,
		})
		require.NoError(t, err)
		defer cancel()

		receiveWithTimeout(t, ch, time.Second)

		assert.Equal(t, http.MethodPost, receivedMethod)
	})
}
