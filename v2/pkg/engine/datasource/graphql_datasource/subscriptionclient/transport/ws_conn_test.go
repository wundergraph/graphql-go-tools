package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

func TestWSConnection_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("returns channel and calls protocol subscribe", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{
			Query: "subscription { test }",
		})
		defer cancel()

		require.NoError(t, err)
		assert.NotNil(t, ch)
		assert.Len(t, proto.SubscribeCalls(), 1)
		assert.Equal(t, "sub-1", proto.SubscribeCalls()[0].ID)
	})

	t.Run("returns error for duplicate subscription id", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		_, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)
		defer cancel()

		_, _, err = wsc.Subscribe(context.Background(), "sub-1", &common.Request{})

		assert.ErrorIs(t, err, transport.ErrSubscriptionExists)
	})

	t.Run("returns error when connection is closed", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)
		wsc.Close()

		_, _, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})

		assert.ErrorIs(t, err, transport.ErrConnectionClosed)
	})

	t.Run("returns error when protocol subscribe fails", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		proto.subscribeErr = assert.AnError
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		_, _, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})

		assert.Error(t, err)
		assert.Equal(t, 0, wsc.SubCount(), "failed subscription should not be registered")
	})
}

func TestWSConnection_ReadLoop(t *testing.T) {
	t.Parallel()

	t.Run("dispatches data message to subscription channel", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)
		defer cancel()

		go wsc.ReadLoop()

		proto.PushMessage(&protocol.Message{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"value": 42}`)},
		})

		msg := receiveWithTimeout(t, ch, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "42")
	})

	t.Run("closes channel on complete message", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch, _, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)

		go wsc.ReadLoop()

		proto.PushMessage(&protocol.Message{
			ID:   "sub-1",
			Type: protocol.MessageComplete,
		})

		// Consume the message (blocking send requires consumer)
		msg := receiveWithTimeout(t, ch, time.Second)
		assert.True(t, msg.Done)

		assertChannelClosed(t, ch)
	})

	t.Run("responds to ping with pong", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		go wsc.ReadLoop()

		proto.PushMessage(&protocol.Message{Type: protocol.MessagePing})

		assert.Eventually(t, func() bool {
			return proto.PongCount() > 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("ignores messages for unknown subscription ids", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)
		defer cancel()

		go wsc.ReadLoop()

		proto.PushMessage(&protocol.Message{
			ID:      "unknown-sub",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"wrong": true}`)},
		})

		proto.PushMessage(&protocol.Message{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"right": true}`)},
		})

		msg := receiveWithTimeout(t, ch, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "right")
	})
}

func TestWSConnection_Unsubscribe(t *testing.T) {
	t.Parallel()

	t.Run("calls protocol unsubscribe and closes channel", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)

		cancel()

		assert.Len(t, proto.UnsubscribeCalls(), 1)
		assert.Equal(t, "sub-1", proto.UnsubscribeCalls()[0])
		assertChannelClosed(t, ch)
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		_, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)

		cancel()
		cancel()
		cancel()

		assert.Len(t, proto.UnsubscribeCalls(), 1)
	})

	t.Run("times out using WriteTimeout", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		proto.unsubscribeDelay = 500 * time.Millisecond
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)
		wsc.WriteTimeout = 50 * time.Millisecond

		_, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)

		start := time.Now()
		cancel()
		elapsed := time.Since(start)

		assert.Less(t, elapsed, 200*time.Millisecond)
	})
}

func TestWSConnection_OnEmpty(t *testing.T) {
	t.Parallel()

	t.Run("calls callback when last subscription removed", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, func() {
			emptyCalled <- struct{}{}
		})

		_, cancel, _ := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		cancel()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(100 * time.Millisecond):
			t.Error("onEmpty callback not called")
		}

		assert.True(t, wsc.IsClosed(), "connection should be closed after last subscription removed")
	})

	t.Run("does not call callback when subscriptions remain", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, func() {
			emptyCalled <- struct{}{}
		})

		_, cancel1, _ := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		_, cancel2, _ := wsc.Subscribe(context.Background(), "sub-2", &common.Request{})

		cancel1()

		select {
		case <-emptyCalled:
			t.Error("onEmpty should not be called when subscriptions remain")
		case <-time.After(100 * time.Millisecond):
			// success
		}

		cancel2()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(100 * time.Millisecond):
			t.Error("onEmpty should be called after last subscription removed")
		}
	})
}

func TestWSConnection_Close(t *testing.T) {
	t.Parallel()

	t.Run("notifies all subscriptions with error", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		ch1, _, _ := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		ch2, _, _ := wsc.Subscribe(context.Background(), "sub-2", &common.Request{})

		wsc.Close()

		// Consume messages (blocking send requires consumer)
		msg1 := receiveWithTimeout(t, ch1, 100*time.Millisecond)
		assert.Error(t, msg1.Err)
		assert.True(t, msg1.Done)

		msg2 := receiveWithTimeout(t, ch2, 100*time.Millisecond)
		assert.Error(t, msg2.Err)
		assert.True(t, msg2.Done)

		assertChannelClosed(t, ch1)
		assertChannelClosed(t, ch2)
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		assert.NoError(t, wsc.Close())
		assert.NoError(t, wsc.Close())
		assert.NoError(t, wsc.Close())
	})
}

func TestWSConnection_SubCount(t *testing.T) {
	t.Parallel()

	t.Run("tracks subscription count accurately", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)

		assert.Equal(t, 0, wsc.SubCount())

		_, cancel1, _ := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		assert.Equal(t, 1, wsc.SubCount())

		_, cancel2, _ := wsc.Subscribe(context.Background(), "sub-2", &common.Request{})
		assert.Equal(t, 2, wsc.SubCount())

		cancel1()
		assert.Equal(t, 1, wsc.SubCount())

		cancel2()
		assert.Equal(t, 0, wsc.SubCount())
	})
}

func TestWSConnection_WriteTimeout(t *testing.T) {
	t.Parallel()

	t.Run("pong write respects WriteTimeout", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		proto.pongDelay = 500 * time.Millisecond
		wsc := transport.NewWSConnection(t.Context(), conn, proto, nil, nil)
		wsc.WriteTimeout = 50 * time.Millisecond

		ch, cancel, err := wsc.Subscribe(context.Background(), "sub-1", &common.Request{})
		require.NoError(t, err)
		defer cancel()

		go wsc.ReadLoop()

		// Send ping (will trigger slow pong)
		proto.PushMessage(&protocol.Message{Type: protocol.MessagePing})

		// Send data message right after
		proto.PushMessage(&protocol.Message{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"test": true}`)},
		})

		// Should receive data within timeout + small buffer
		// If pong blocked for 500ms, this would timeout
		msg := receiveWithTimeout(t, ch, 150*time.Millisecond)
		assert.NotNil(t, msg.Payload)
	})
}

// Test helpers

func newTestConn(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	serverConn := make(chan *websocket.Conn, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		serverConn <- conn

		for {
			_, _, err := conn.Read(r.Context())
			if err != nil {
				conn.Close(websocket.StatusNormalClosure, "shutdown")
				return
			}
		}
	}))

	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), url, nil)
	require.NoError(t, err)

	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "shutdown") })

	srvConn := <-serverConn
	t.Cleanup(func() { srvConn.CloseNow() })

	return clientConn, srvConn
}

func assertChannelClosed(t *testing.T, ch <-chan *common.Message) {
	t.Helper()
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed")
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel to close")
	}
}

// mockProtocol implements protocol.Protocol for testing.
type mockProtocol struct {
	mu               sync.Mutex
	subscribeCalls   []subscribeCall
	unsubCalls       []string
	pongCount        int
	subscribeErr     error
	unsubscribeDelay time.Duration
	pongDelay        time.Duration

	messages chan *protocol.Message
}

type subscribeCall struct {
	ID  string
	Req *common.Request
}

func newMockProtocol() *mockProtocol {
	return &mockProtocol{
		messages: make(chan *protocol.Message, 100),
	}
}

func (m *mockProtocol) Subprotocol() string { return "graphql-transport-ws" }

func (m *mockProtocol) Init(ctx context.Context, conn *websocket.Conn, payload map[string]any) error {
	return nil
}

func (m *mockProtocol) Subscribe(ctx context.Context, conn *websocket.Conn, id string, req *common.Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribeCalls = append(m.subscribeCalls, subscribeCall{ID: id, Req: req})
	return m.subscribeErr
}

func (m *mockProtocol) Unsubscribe(ctx context.Context, conn *websocket.Conn, id string) error {
	if m.unsubscribeDelay > 0 {
		select {
		case <-time.After(m.unsubscribeDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsubCalls = append(m.unsubCalls, id)
	return nil
}

func (m *mockProtocol) Read(ctx context.Context, conn *websocket.Conn) (*protocol.Message, error) {
	select {
	case msg := <-m.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *mockProtocol) Ping(ctx context.Context, conn *websocket.Conn) error {
	return nil
}

func (m *mockProtocol) Pong(ctx context.Context, conn *websocket.Conn) error {
	if m.pongDelay > 0 {
		select {
		case <-time.After(m.pongDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pongCount++
	return nil
}

func (m *mockProtocol) PushMessage(msg *protocol.Message) {
	m.messages <- msg
}

func (m *mockProtocol) SubscribeCalls() []subscribeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]subscribeCall{}, m.subscribeCalls...)
}

func (m *mockProtocol) UnsubscribeCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.unsubCalls...)
}

func (m *mockProtocol) PongCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pongCount
}
