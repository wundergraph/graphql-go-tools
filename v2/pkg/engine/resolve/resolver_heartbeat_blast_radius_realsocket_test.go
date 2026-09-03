package resolve

import (
	"bufio"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// tcpFlushWriter is a SubscriptionResponseWriter backed by a real TCP
// connection. Write buffers in-process (via a bufio.Writer sized comfortably
// larger than any payload used in the test, so Write itself never touches
// the network); Flush is the only call that reaches the OS, mirroring
// cosmo-router's real HttpFlushWriter (core/subscription_response_writer.go)
// and the exact call site that blocks in production: resolve.go:1077's
// `sub.writer.Flush()` inside executeSubscriptionUpdate.
type tcpFlushWriter struct {
	bw *bufio.Writer
}

func newTCPFlushWriter(conn net.Conn) *tcpFlushWriter {
	return &tcpFlushWriter{bw: bufio.NewWriterSize(conn, 8*1024*1024)}
}

func (w *tcpFlushWriter) Write(p []byte) (int, error) { return w.bw.Write(p) }
func (w *tcpFlushWriter) Flush() error                { return w.bw.Flush() }
func (w *tcpFlushWriter) Complete()                   {}
func (w *tcpFlushWriter) Error([]byte)                {}
func (w *tcpFlushWriter) Heartbeat() error {
	if _, err := w.bw.Write([]byte("H")); err != nil {
		return err
	}
	return w.bw.Flush()
}

var _ SubscriptionResponseWriter = (*tcpFlushWriter)(nil)

// TestResolver_RealDeadTCPClientDoesNotBlockHeartbeatForUnrelatedTrigger is
// the real-transport sibling of
// TestResolver_StuckWriteDoesNotBlockHeartbeatForUnrelatedTrigger. Instead of
// locking writeMu directly to simulate a stuck write, it uses a real TCP
// connection to a "client" that stops reading without closing the socket --
// the exact scenario caught live in production -- so the server's actual
// Flush() call genuinely blocks at the OS level, not a simulated stand-in,
// and proves the fix (sendHeartbeat's TryLock) still decouples an unrelated
// trigger's heartbeat from that real block, without claiming the stuck write
// itself is in any way resolved by the fix -- it stays genuinely blocked
// throughout, exactly as a real dead connection would.
func TestResolver_RealDeadTCPClientDoesNotBlockHeartbeatForUnrelatedTrigger(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverConnCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			serverConnCh <- conn
		}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer clientConn.Close()

	// Shrink the client's receive buffer so the server's real write blocks
	// after a modest payload instead of requiring an enormous one -- then
	// never read from it again below, exactly like a client that silently
	// stops draining its socket without ever closing the connection.
	if tcpConn, ok := clientConn.(*net.TCPConn); ok {
		_ = tcpConn.SetReadBuffer(1024)
	}

	var serverConn net.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(time.Second):
		t.Fatal("server never accepted connection")
	}
	defer serverConn.Close()

	resolverCtx := t.Context()
	resolver := New(resolverCtx, ResolverOptions{
		MaxConcurrency:                1,
		AsyncErrorWriter:              &FakeErrorWriter{},
		SubscriptionHeartbeatInterval: time.Hour, // long interval so the background loop doesn't compete
	})

	const stuckTriggerID = uint64(1)
	const healthyTriggerID = uint64(2)

	deadClientWriter := newTCPFlushWriter(serverConn)
	subA := &subscriptionState{
		triggerID: stuckTriggerID,
		ctx:       NewContext(context.Background()),
		writer:    deadClientWriter,
		id:        SubscriptionIdentifier{ConnectionID: 1, SubscriptionID: 1},
		heartbeat: true,
		completed: make(chan struct{}),
	}

	heartbeatReceived := make(chan struct{})
	var closeOnce sync.Once
	subB := &subscriptionState{
		triggerID: healthyTriggerID,
		ctx:       NewContext(context.Background()),
		writer: &RecordingHeartbeatWriter{onHeartbeat: func() {
			closeOnce.Do(func() { close(heartbeatReceived) })
		}},
		id:        SubscriptionIdentifier{ConnectionID: 2, SubscriptionID: 2},
		heartbeat: true,
		completed: make(chan struct{}),
	}

	resolver.mu.Lock()
	resolver.triggers[stuckTriggerID] = &trigger{
		id:            stuckTriggerID,
		cancel:        func() {},
		subscriptions: map[SubscriptionIdentifier]*subscriptionState{subA.id: subA},
	}
	resolver.triggers[healthyTriggerID] = &trigger{
		id:            healthyTriggerID,
		cancel:        func() {},
		subscriptions: map[SubscriptionIdentifier]*subscriptionState{subB.id: subB},
	}
	resolver.mu.Unlock()

	// Simulate a real subscription push exactly as executeSubscriptionUpdate
	// does at resolve.go:1041-1084: acquire writeMu, write a payload well
	// beyond the shrunk client window plus any OS send buffer, then Flush --
	// a real net.Conn write that genuinely blocks because nothing on the
	// other end is draining it.
	writeStarted := make(chan struct{})
	writeUnblocked := make(chan struct{})
	go func() {
		subA.writeMu.Lock()
		defer subA.writeMu.Unlock()
		close(writeStarted)
		payload := make([]byte, 4*1024*1024)
		_, _ = deadClientWriter.Write(payload)
		_ = deadClientWriter.Flush() // blocks here until the client reads or the conn closes
		close(writeUnblocked)
	}()
	<-writeStarted

	done := make(chan struct{})
	go func() {
		// Mirrors sendTriggerHeartbeats' own sequential loop, order pinned
		// for determinism -- see the mutex-based sibling test for why.
		resolver.heartbeatTriggerSubscriptions(stuckTriggerID)
		resolver.heartbeatTriggerSubscriptions(healthyTriggerID)
		close(done)
	}()

	// Even with the real write still genuinely stuck, trigger 2's heartbeat
	// must fire promptly -- sendHeartbeat's TryLock skips subA's contended
	// heartbeat instead of blocking the sweep behind it.
	select {
	case <-heartbeatReceived:
		// expected: the fix means this doesn't wait on the real stuck write.
	case <-writeUnblocked:
		t.Fatal("the real TCP write unblocked on its own — the client-side buffer wasn't shrunk enough to force a genuine block; test setup needs a larger payload or smaller receive buffer")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("healthy trigger's heartbeat did not fire promptly — stuck write is still blocking the sweep, fix not effective")
	}

	// The real write itself is unaffected by the fix -- it's still
	// genuinely blocked at this point, exactly as a real dead connection
	// would be. The fix only decouples the heartbeat sweep from it.
	select {
	case <-writeUnblocked:
		t.Fatal("the stuck write unblocked before the client ever read anything — test setup invalid")
	default:
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat sweep never completed")
	}

	// Clean up: let the client actually start draining, confirming the
	// underlying stuck write resolves normally once a real client would
	// (or the connection is torn down).
	go io.Copy(io.Discard, clientConn)

	select {
	case <-writeUnblocked:
	case <-time.After(2 * time.Second):
		t.Fatal("the stuck write itself never unblocked")
	}
}
