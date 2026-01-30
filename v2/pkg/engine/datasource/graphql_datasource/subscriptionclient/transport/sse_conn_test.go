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
)

func TestSSEConnection_ReadLoop(t *testing.T) {
	t.Run("reads and parses SSE events", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(
			"event: next\ndata: {\"data\":{\"time\":\"12:00\"}}\n\n",
		))
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		go conn.ReadLoop()

		msg := <-conn.ch
		require.NotNil(t, msg.Payload)
		// Data field contains the raw "data" value from GraphQL response
		assert.JSONEq(t, `{"time":"12:00"}`, string(msg.Payload.Data))
	})

	t.Run("closes channel on EOF", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		go conn.ReadLoop()

		select {
		case _, ok := <-conn.ch:
			assert.False(t, ok, "channel should be closed")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("channel not closed on EOF")
		}
	})

	t.Run("sends error on read failure", func(t *testing.T) {
		body := &errorReader{err: io.ErrUnexpectedEOF}
		resp := &http.Response{Body: io.NopCloser(body)}
		conn := newSSEConnection(resp)

		go conn.ReadLoop()

		msg := <-conn.ch
		assert.Error(t, msg.Err)
		assert.True(t, msg.Done)
	})

	t.Run("stops on complete event", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(
			"event: next\ndata: {\"data\":{}}\n\n" +
				"event: complete\ndata:\n\n" +
				"event: next\ndata: {\"data\":{}}\n\n", // Should not receive this
		))
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		go conn.ReadLoop()

		// First message
		msg1 := <-conn.ch
		assert.NotNil(t, msg1.Payload)
		assert.False(t, msg1.Done)

		// Complete message
		msg2 := <-conn.ch
		assert.True(t, msg2.Done)

		// Channel should close, no third message
		select {
		case _, ok := <-conn.ch:
			assert.False(t, ok, "channel should be closed after complete")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("channel not closed after complete")
		}
	})
}

func TestSSEConnection_Close(t *testing.T) {
	t.Run("closes channel and body", func(t *testing.T) {
		pr, pw := io.Pipe()
		body := &trackingCloser{Reader: pr}
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		go conn.ReadLoop()

		err := conn.Close()
		require.NoError(t, err)
		pw.Close() // Ensure pipe is fully closed

		// Channel close signals cleanup completed
		select {
		case _, ok := <-conn.ch:
			require.False(t, ok, "channel should be closed")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("channel should be closed (timeout)")
		}

		assert.True(t, body.closed.Load(), "body should be closed")
	})

	t.Run("is idempotent", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		err1 := conn.Close()
		err2 := conn.Close()

		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

}

func TestSSEConnection_Channel(t *testing.T) {
	t.Run("returns buffered channel", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		resp := &http.Response{Body: body}
		conn := newSSEConnection(resp)

		ch := conn.ch
		assert.NotNil(t, ch)
		assert.Equal(t, 8, cap(ch))
	})
}

// errorReader always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

// trackingCloser tracks if Close was called
type trackingCloser struct {
	io.Reader
	closed atomic.Bool
}

func (c *trackingCloser) Close() error {
	c.closed.Store(true)
	return nil
}
