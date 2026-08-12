package transport

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

func TestSSEConnection_ReadLoop(t *testing.T) {
	t.Run("reads and parses SSE events", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(
			"event: next\ndata: {\"data\":{\"time\":\"12:00\"}}\n\n",
		))
		resp := &http.Response{Body: body}
		handler, receive := collectingHandler()
		conn := newSSEConnection(resp, handler, nil)

		go conn.readLoop()

		msg := receive(t, 1*time.Second)
		require.NotNil(t, msg.Payload)
		// Data field contains the raw "data" value from GraphQL response
		assert.JSONEq(t, `{"time":"12:00"}`, string(msg.Payload.Data))
	})

	t.Run("delivers connection error on EOF", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		resp := &http.Response{Body: body}
		handler, receive := collectingHandler()
		conn := newSSEConnection(resp, handler, nil)

		go conn.readLoop()

		msg := receive(t, 1*time.Second)
		assert.Equal(t, common.MessageTypeConnectionError, msg.Type)
		assert.Error(t, msg.Err)
	})

	t.Run("sends error on read failure", func(t *testing.T) {
		body := &errorReader{err: io.ErrUnexpectedEOF}
		resp := &http.Response{Body: io.NopCloser(body)}
		handler, receive := collectingHandler()
		conn := newSSEConnection(resp, handler, nil)

		go conn.readLoop()

		msg := receive(t, 1*time.Second)
		require.Error(t, msg.Err)
		assert.Equal(t, common.MessageTypeConnectionError, msg.Type)
	})

	t.Run("stops on complete event", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(
			"event: next\ndata: {\"data\":{}}\n\n" +
				"event: complete\ndata:\n\n" +
				"event: next\ndata: {\"data\":{}}\n\n", // Should not receive this
		))
		resp := &http.Response{Body: body}
		handler, receive := collectingHandler()
		wrappedHandler, collect := waitForMessages(handler)
		conn := newSSEConnection(resp, wrappedHandler, nil)

		go conn.readLoop()

		// First message
		msg1 := receive(t, 1*time.Second)
		assert.NotNil(t, msg1.Payload)
		assert.Equal(t, common.MessageTypeData, msg1.Type)

		// Complete message
		msg2 := receive(t, 1*time.Second)
		assert.Equal(t, common.MessageTypeComplete, msg2.Type)

		// Wait and verify no more messages arrive after complete
		messages := collect(100 * time.Millisecond)
		assert.Len(t, messages, 2, "should receive exactly 2 messages before stopping")
	})
}

func TestSSEConnection_Close(t *testing.T) {
	t.Run("closes body", func(t *testing.T) {
		pr, pw := io.Pipe()
		body := &trackingCloser{Reader: pr}
		resp := &http.Response{Body: body}
		handler, _ := collectingHandler()
		conn := newSSEConnection(resp, handler, nil)

		go conn.readLoop()

		conn.closeConn()
		pw.Close() // Ensure pipe is fully closed

		assert.Eventually(t, func() bool {
			return body.closed.Load()
		}, 1*time.Second, 10*time.Millisecond, "body should be closed")
	})

	t.Run("is idempotent", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		resp := &http.Response{Body: body}
		handler, _ := collectingHandler()
		conn := newSSEConnection(resp, handler, nil)

		conn.closeConn()
		conn.closeConn() // second call is a no-op
	})
}

// errorReader always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// trackingCloser tracks if Close was called and forwards to the underlying reader if it implements io.Closer.
type trackingCloser struct {
	io.Reader

	closed atomic.Bool
}

func (c *trackingCloser) Close() error {
	c.closed.Store(true)
	if closer, ok := c.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
