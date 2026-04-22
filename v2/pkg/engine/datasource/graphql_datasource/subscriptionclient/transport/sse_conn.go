package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/r3labs/sse/v2"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

var (
	headerData  = []byte("data:")
	headerEvent = []byte("event:")
)

// sseConnection handles a single SSE subscription stream.
type sseConnection struct {
	resp    *http.Response
	handler common.Handler
	closed  atomic.Bool
}

func newSSEConnection(resp *http.Response, handler common.Handler) *sseConnection {
	return &sseConnection{
		resp:    resp,
		handler: handler,
	}
}

// readLoop reads SSE events from the response body and delivers them to the handler.
// Every exit path delivers a terminal message to the handler unless the connection
// was closed by the consumer.
func (c *sseConnection) readLoop() {
	defer c.cleanup()

	reader := sse.NewEventStreamReader(c.resp.Body, 1<<16) // 64KB

	for {
		if c.closed.Load() {
			return
		}

		eventBytes, err := reader.ReadEvent()
		if err != nil {
			if c.closed.Load() {
				return
			}
			c.sendError(err)
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
		c.handler(msg)

		if msg.Type.IsTerminal() {
			return
		}
	}
}

// parseEventBytes extracts the event type and data from raw SSE event bytes.
// Based on r3labs/sse's processEvent but simplified for our needs.
func (c *sseConnection) parseEventBytes(msg []byte) (eventType string, data []byte) {
	if len(msg) == 0 {
		return "", nil
	}

	for line := range bytes.Lines(msg) {
		line = bytes.TrimRight(line, "\r\n")
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
func (c *sseConnection) parseEvent(eventType string, data []byte) *common.Message {
	switch eventType {
	case "next":
		var resp common.ExecutionResult
		if err := json.Unmarshal(data, &resp); err != nil {
			return &common.Message{Type: common.MessageTypeConnectionError, Err: err}
		}
		return &common.Message{Type: common.MessageTypeData, Payload: &resp}

	case "error":
		return &common.Message{
			Type:    common.MessageTypeError,
			Payload: &common.ExecutionResult{Errors: data},
		}

	case "complete":
		return &common.Message{Type: common.MessageTypeComplete}

	default:
		// Unknown event type or no event type specified - treat as data
		// This handles servers that send data without an event type
		if len(data) == 0 {
			return &common.Message{Type: common.MessageTypeComplete}
		}
		var resp common.ExecutionResult
		if err := json.Unmarshal(data, &resp); err != nil {
			return &common.Message{Type: common.MessageTypeConnectionError, Err: err}
		}
		return &common.Message{Type: common.MessageTypeData, Payload: &resp}
	}
}

func (c *sseConnection) sendError(err error) {
	if c.closed.Load() {
		return
	}
	c.handler(&common.Message{Type: common.MessageTypeConnectionError, Err: err})
}

func (c *sseConnection) cleanup() {
	c.closed.Store(true)

	c.resp.Body.Close()
}

// closeConn terminates the SSE connection.
func (c *sseConnection) closeConn() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	c.resp.Body.Close()
}
