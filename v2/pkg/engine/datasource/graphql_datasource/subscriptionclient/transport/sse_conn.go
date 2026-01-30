package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/r3labs/sse/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

var (
	headerData  = []byte("data:")
	headerEvent = []byte("event:")
)

// SSEConnection handles a single SSE subscription stream.
type SSEConnection struct {
	resp   *http.Response
	ch     chan *common.Message
	done   chan struct{}
	closed atomic.Bool
}

func newSSEConnection(resp *http.Response) *SSEConnection {
	return &SSEConnection{
		resp: resp,
		ch:   make(chan *common.Message, 8),
		done: make(chan struct{}),
	}
}

// readLoop reads SSE events from the response body and sends them to the channel.
func (c *SSEConnection) ReadLoop() {
	defer c.cleanup()

	reader := sse.NewEventStreamReader(c.resp.Body, 1<<16) // 64KB

	for {
		if c.closed.Load() {
			return
		}

		eventBytes, err := reader.ReadEvent()
		if err != nil {
			if err != io.EOF {
				c.sendError(err)
			}
			return
		}

		// Parse the raw event bytes into event type and data
		eventType, data := c.parseEventBytes(eventBytes)

		// Skip empty events (e.g., keep-alive comments)
		if eventType == "" && data == nil {
			continue
		}

		msg := c.parseEvent(eventType, data)

		if c.closed.Load() {
			return
		}
		select {
		case c.ch <- msg:
		case <-c.done:
			return
		}

		if msg.Done {
			return
		}
	}
}

// parseEventBytes extracts the event type and data from raw SSE event bytes.
// Based on r3labs/sse's processEvent but simplified for our needs.
func (c *SSEConnection) parseEventBytes(msg []byte) (eventType string, data []byte) {
	if len(msg) == 0 {
		return "", nil
	}

	// Split by newlines (normalize CR/LF)
	for _, line := range bytes.FieldsFunc(msg, func(r rune) bool { return r == '\n' || r == '\r' }) {
		switch {
		case bytes.HasPrefix(line, headerEvent):
			eventType = string(trimHeader(len(headerEvent), line))

		case bytes.HasPrefix(line, headerData):
			// The spec allows for multiple data fields per event, concatenated with "\n"
			data = append(data, trimHeader(len(headerData), line)...)
			data = append(data, '\n')

		case bytes.Equal(line, []byte("data")):
			// A line that simply contains "data" should be treated as empty data
			data = append(data, '\n')

			// Comments (lines starting with ':') are already filtered by EventStreamReader
		}
	}

	// Trim the trailing "\n" per SSE spec
	data = bytes.TrimSuffix(data, []byte("\n"))

	return eventType, data
}

// trimHeader removes the header prefix and optional leading space.
func trimHeader(size int, data []byte) []byte {
	if len(data) < size {
		return data
	}

	data = data[size:]
	// Remove optional leading whitespace (single space after colon)
	if len(data) > 0 && data[0] == ' ' {
		data = data[1:]
	}
	return data
}

// parseEvent converts parsed SSE event data into a shared.Message.
func (c *SSEConnection) parseEvent(eventType string, data []byte) *common.Message {
	switch eventType {
	case "next":
		var resp common.ExecutionResult
		if err := json.Unmarshal(data, &resp); err != nil {
			return &common.Message{
				Err:  err,
				Done: true,
			}
		}
		return &common.Message{Payload: &resp}

	case "error":
		var errors []common.GraphQLError
		if err := json.Unmarshal(data, &errors); err != nil {
			return &common.Message{
				Err:  err,
				Done: true,
			}
		}
		return &common.Message{
			Err:  &common.SubscriptionError{Errors: errors},
			Done: true,
		}

	case "complete":
		return &common.Message{Done: true}

	default:
		// Unknown event type or no event type specified - treat as data
		// This handles servers that send data without an event type
		if len(data) == 0 {
			return &common.Message{Done: true}
		}
		var resp common.ExecutionResult
		if err := json.Unmarshal(data, &resp); err != nil {
			return &common.Message{
				Err:  err,
				Done: true,
			}
		}
		return &common.Message{Payload: &resp}
	}
}

func (c *SSEConnection) sendError(err error) {
	if c.closed.Load() {
		return
	}
	select {
	case c.ch <- &common.Message{Err: err, Done: true}:
	case <-c.done:
	}
}

func (c *SSEConnection) cleanup() {
	c.closed.Store(true)

	c.resp.Body.Close()
	close(c.ch) // Close channel so fanout exits
}

// Close terminates the SSE connection.
func (c *SSEConnection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(c.done)
	c.resp.Body.Close()

	return nil
}
