package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
)

func TestWSConnection_Subscribe(t *testing.T) {
	t.Parallel()

	t.Run("calls protocol subscribe and handler can receive", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		handler, _ := collectingHandler()
		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{
			Query: "subscription { test }",
		}, handler)
		require.NoError(t, err)
		defer cancel()
		assert.Len(t, proto.SubscribeCalls(), 1)
		assert.Equal(t, "sub-1", proto.SubscribeCalls()[0].ID)
	})

	t.Run("returns error for duplicate subscription id", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)
		defer cancel()

		_, err = wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})

		assert.ErrorIs(t, err, ErrSubscriptionExists)
	})

	t.Run("returns error when connection is closed", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})
		wsc.closeConn()

		_, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})

		assert.ErrorIs(t, err, common.ErrConnectionClosed)
	})

	t.Run("returns ErrConnectionClosed when shutdown races before lock acquisition", func(t *testing.T) {
		t.Parallel()

		// Test for TOCTOU between closed.Load() and subsMu.Lock()
		// in subscribe().
		//
		// Without a closed re-check under the lock, this sequence is possible:
		// 1. subscribe: closed.Load() → false  (check)
		// 2. shutdown:  closed.CAS(false,true)  (invalidates the check)
		// 3. shutdown:  swaps c.subs, dispatches errors to old handlers
		// 4. subscribe: subsMu.Lock(), adds handler to NEW empty map (use)
		// 5. subscribe: protocol.Subscribe → nil (mock doesn't check conn)
		// 6. subscribe returns (cancel, nil) — looks successful
		//
		// The handler is now orphaned: closed=true prevents any future
		// shutdown from running (CAS fails), so the handler will never
		// receive a terminal message. This test forces the exact interleaving
		// deterministically.

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		// Step 1: Hold subsMu so subscribe() blocks after its closed check.
		wsc.subsMu.Lock()

		subscribeResult := make(chan error, 1)
		go func() {
			// Passes closed.Load() (still false), then blocks on subsMu.Lock().
			_, err := wsc.subscribe(context.Background(), "sub-race", &common.Request{}, func(_ *common.Message) {})
			subscribeResult <- err
		}()

		// Let the goroutine reach the blocked Lock().
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)

		// Step 2: Simulate what shutdown does first — set closed=true.
		// We set it directly rather than calling shutdown() because shutdown
		// also needs subsMu (which we hold), and we want to control ordering.
		wsc.closed.Store(true)

		// Step 3: Release the lock. subscribe() now enters the critical
		// section with a stale view: it saw closed=false, but closed is now true.
		wsc.subsMu.Unlock()

		select {
		case err := <-subscribeResult:
			// With the fix: subscribe re-checks closed under the lock → ErrConnectionClosed.
			// Without the fix: subscribe succeeds (nil error) — the handler is orphaned
			// because closed=true means no future shutdown() can deliver a terminal message.
			require.ErrorIs(t, err, common.ErrConnectionClosed,
				"subscribe must detect closed state under the lock; without this check "+
					"the handler is orphaned (closed=true prevents future shutdown)")
		case <-time.After(time.Second):
			t.Fatal("subscribe did not return within timeout")
		}
	})

	t.Run("returns error when protocol subscribe fails", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		proto.subscribeErr = assert.AnError
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		_, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})

		assert.Error(t, err)
		assert.Equal(t, 0, wsc.subCount(), "failed subscription should not be registered")
	})
}

func TestWSConnection_ReadLoop(t *testing.T) {
	t.Parallel()

	t.Run("dispatches data message to subscription handler", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		handler, receive := collectingHandler()
		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, handler)
		require.NoError(t, err)
		defer cancel()

		go wsc.readLoop()

		proto.PushMessage(&protocol.WireMessage{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"value": 42}`)},
		})

		msg := receive(t, time.Second)
		require.NotNil(t, msg.Payload)
		assert.Contains(t, string(msg.Payload.Data), "42")
	})

	t.Run("delivers complete message to handler", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		handler, receive := collectingHandler()
		_, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, handler)
		require.NoError(t, err)

		go wsc.readLoop()

		proto.PushMessage(&protocol.WireMessage{
			ID:   "sub-1",
			Type: protocol.MessageComplete,
		})

		msg := receive(t, time.Second)
		assert.Equal(t, common.MessageTypeComplete, msg.Type)
	})

	t.Run("responds to ping with pong", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		go wsc.readLoop()

		proto.PushMessage(&protocol.WireMessage{Type: protocol.MessagePing})

		assert.Eventually(t, func() bool {
			return proto.PongCount() > 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("ignores messages for unknown subscription ids", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		handler, receive := collectingHandler()
		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, handler)
		require.NoError(t, err)
		defer cancel()

		go wsc.readLoop()

		proto.PushMessage(&protocol.WireMessage{
			ID:      "unknown-sub",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"wrong": true}`)},
		})

		proto.PushMessage(&protocol.WireMessage{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"right": true}`)},
		})

		msg := receive(t, time.Second)
		assert.Contains(t, string(msg.Payload.Data), "right")
	})
}

func TestWSConnection_Unsubscribe(t *testing.T) {
	t.Parallel()

	t.Run("calls protocol unsubscribe and removes subscription", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel()

		assert.Len(t, proto.UnsubscribeCalls(), 1)
		assert.Equal(t, "sub-1", proto.UnsubscribeCalls()[0])
		assert.Equal(t, 0, wsc.subCount())
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
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
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			writeTimeout: 50 * time.Millisecond,
		})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
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
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			onEmpty: func() { emptyCalled <- struct{}{} },
		})

		cancel, _ := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		cancel()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(100 * time.Millisecond):
			t.Error("onEmpty callback not called")
		}

		assert.True(t, wsc.isClosed(), "connection should be closed after last subscription removed")
	})

	t.Run("does not call callback when subscriptions remain", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			onEmpty: func() { emptyCalled <- struct{}{} },
		})

		cancel1, _ := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		cancel2, _ := wsc.subscribe(context.Background(), "sub-2", &common.Request{}, func(_ *common.Message) {})

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

	t.Run("calls callback on direct Close", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			onEmpty: func() { emptyCalled <- struct{}{} },
		})

		wsc.closeConn()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(100 * time.Millisecond):
			t.Error("onEmpty callback not called on Close")
		}
	})

	t.Run("calls callback on read loop exit", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			onEmpty: func() { emptyCalled <- struct{}{} },
		})

		go wsc.readLoop()

		// Close the connection to cause the read loop to exit
		wsc.closeConn()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(time.Second):
			t.Error("onEmpty callback not called on read loop exit")
		}
	})
}

func TestWSConnection_IdleTimeout(t *testing.T) {
	t.Parallel()

	t.Run("removeSub defers close for idle timeout duration when subs are empty", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			idleTimeout: 200 * time.Millisecond,
		})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel()

		assert.False(t, wsc.isClosed(), "connection should not be closed immediately")

		assert.Eventually(t, func() bool {
			return wsc.isClosed()
		}, time.Second, 10*time.Millisecond, "connection should close after idle timeout")
	})

	t.Run("removeSub does not close when new subscription arrives before timeout", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			idleTimeout: 200 * time.Millisecond,
		})

		cancel1, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel1() // starts idle timer

		_, err = wsc.subscribe(context.Background(), "sub-2", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)

		time.Sleep(300 * time.Millisecond)

		assert.False(t, wsc.isClosed(), "connection should stay open while subscription exists")
	})

	t.Run("removeSub closes immediately when idle timeout is zero", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()

		emptyCalled := make(chan struct{}, 1)
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			onEmpty: func() { emptyCalled <- struct{}{} },
		})

		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		require.NoError(t, err)

		cancel()

		select {
		case <-emptyCalled:
			// success
		case <-time.After(100 * time.Millisecond):
			t.Error("connection should close immediately with zero idle timeout")
		}

		assert.True(t, wsc.isClosed())
	})

}

func TestWSConnection_Close(t *testing.T) {
	t.Parallel()

	t.Run("notifies all subscriptions with error", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		handler1, receive1 := collectingHandler()
		_, _ = wsc.subscribe(context.Background(), "sub-1", &common.Request{}, handler1)

		handler2, receive2 := collectingHandler()
		_, _ = wsc.subscribe(context.Background(), "sub-2", &common.Request{}, handler2)

		wsc.closeConn()

		msg1 := receive1(t, 100*time.Millisecond)
		assert.Error(t, msg1.Err)
		assert.Equal(t, common.MessageTypeConnectionError, msg1.Type)

		msg2 := receive2(t, 100*time.Millisecond)
		assert.Error(t, msg2.Err)
		assert.Equal(t, common.MessageTypeConnectionError, msg2.Type)
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		assert.NotPanics(t, func() {
			wsc.closeConn()
			wsc.closeConn()
			wsc.closeConn()
		})
	})
}

func TestWSConnection_SubCount(t *testing.T) {
	t.Parallel()

	t.Run("tracks subscription count accurately", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		assert.Equal(t, 0, wsc.subCount())

		cancel1, _ := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, func(_ *common.Message) {})
		assert.Equal(t, 1, wsc.subCount())

		cancel2, _ := wsc.subscribe(context.Background(), "sub-2", &common.Request{}, func(_ *common.Message) {})
		assert.Equal(t, 2, wsc.subCount())

		cancel1()
		assert.Equal(t, 1, wsc.subCount())

		cancel2()
		assert.Equal(t, 0, wsc.subCount())
	})
}

func TestWSConnection_WriteTimeout(t *testing.T) {
	t.Parallel()

	t.Run("pong write respects WriteTimeout", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		proto.pongDelay = 500 * time.Millisecond
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			writeTimeout: 50 * time.Millisecond,
		})

		handler, receive := collectingHandler()
		cancel, err := wsc.subscribe(context.Background(), "sub-1", &common.Request{}, handler)
		require.NoError(t, err)
		defer cancel()

		go wsc.readLoop()

		// Send ping (will trigger slow pong)
		proto.PushMessage(&protocol.WireMessage{Type: protocol.MessagePing})

		// Send data message right after
		proto.PushMessage(&protocol.WireMessage{
			ID:      "sub-1",
			Type:    protocol.MessageData,
			Payload: &common.ExecutionResult{Data: json.RawMessage(`{"test": true}`)},
		})

		// Should receive data within timeout + small buffer
		// If pong blocked for 500ms, this would timeout
		msg := receive(t, 150*time.Millisecond)
		assert.NotNil(t, msg.Payload)
	})
}

func TestWSConnection_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("applies default write timeout when omitted", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{})

		assert.Equal(t, defaultWriteTimeout, wsc.writeTimeout)
	})

	t.Run("applies default write timeout for zero value", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			writeTimeout: 0,
		})

		assert.Equal(t, defaultWriteTimeout, wsc.writeTimeout)
	})

	t.Run("overrides write timeout when provided", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			writeTimeout: 10 * time.Second,
		})

		assert.Equal(t, 10*time.Second, wsc.writeTimeout)
	})

	t.Run("ignores negative write timeout", func(t *testing.T) {
		t.Parallel()

		conn, _ := newTestConn(t)
		proto := newMockProtocol()
		wsc := newWSConnection(conn, proto, wsConnectionOptions{
			writeTimeout: -1 * time.Second,
		})

		assert.Equal(t, defaultWriteTimeout, wsc.writeTimeout)
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
	clientConn, _, err := websocket.Dial(context.Background(), url, nil) //nolint:bodyclose
	require.NoError(t, err)

	t.Cleanup(func() {
		clientConn.Close(websocket.StatusNormalClosure, "shutdown")
	})

	srvConn := <-serverConn
	t.Cleanup(func() { _ = srvConn.CloseNow() })

	return clientConn, srvConn
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

	messages chan *protocol.WireMessage
}

type subscribeCall struct {
	ID  string
	Req *common.Request
}

func newMockProtocol() *mockProtocol {
	return &mockProtocol{
		messages: make(chan *protocol.WireMessage, 100),
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

func (m *mockProtocol) Read(ctx context.Context, conn *websocket.Conn) (*protocol.WireMessage, error) {
	select {
	case msg := <-m.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Ping and Pong implement protocol.Pinger — the mock simulates graphql-transport-ws.

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

func (m *mockProtocol) PushMessage(msg *protocol.WireMessage) {
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
