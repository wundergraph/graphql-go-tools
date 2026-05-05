package transport

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
)

// newTestWSConnection mirrors the defaults applied by client.New so that tests
// can continue to pass wsConnectionOptions{...} literals without repeating
// default values at every call site.
func newTestWSConnection(t *testing.T, conn *websocket.Conn, proto protocol.Protocol, opts wsConnectionOptions) *wsConnection {
	t.Helper()
	if opts.logger == nil {
		opts.logger = abstractlogger.NoopLogger
	}
	if opts.writeTimeout <= 0 {
		opts.writeTimeout = 5 * time.Second
	}
	return newWSConnection(conn, proto, opts)
}

// newTestWSTransport mirrors the defaults applied by client.New so that tests
// can continue to pass WSTransportOptions{...} literals without repeating
// default values at every call site.
func newTestWSTransport(t *testing.T, opts WSTransportOptions) *WSTransport {
	t.Helper()
	if opts.UpgradeClient == nil {
		opts.UpgradeClient = http.DefaultClient
	}
	if opts.Logger == nil {
		opts.Logger = abstractlogger.NoopLogger
	}
	if opts.ReadLimit <= 0 {
		opts.ReadLimit = 1 << 20
	}
	if opts.AckTimeout <= 0 {
		opts.AckTimeout = 30 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 5 * time.Second
	}
	return NewWSTransport(t.Context(), opts)
}

// collectingHandler returns a handler that appends messages to a channel,
// plus a helper to receive with timeout (for use in tests).
func collectingHandler() (common.Handler, func(t *testing.T, timeout time.Duration) *common.Message) {
	ch := make(chan *common.Message, 64)
	handler := func(msg *common.Message) {
		ch <- msg
	}
	receive := func(t *testing.T, timeout time.Duration) *common.Message {
		t.Helper()
		select {
		case msg := <-ch:
			return msg
		case <-time.After(timeout):
			t.Fatal("timeout waiting for message")
			return nil
		}
	}
	return handler, receive
}

// waitForMessages collects messages from a handler until a terminal message or timeout.
func waitForMessages(handler common.Handler) (common.Handler, func(timeout time.Duration) []*common.Message) {
	var mu sync.Mutex
	var msgs []*common.Message
	done := make(chan struct{}, 1)

	wrappedHandler := func(msg *common.Message) {
		mu.Lock()
		msgs = append(msgs, msg)
		mu.Unlock()
		handler(msg)
		if msg.Type.IsTerminal() {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}

	collect := func(timeout time.Duration) []*common.Message {
		select {
		case <-done:
		case <-time.After(timeout):
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]*common.Message{}, msgs...)
	}

	return wrappedHandler, collect
}
