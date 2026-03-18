package transport

import (
	"sync"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

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
