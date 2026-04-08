package common

import (
	"encoding/json"
	"errors"
)

var ErrConnectionClosed = errors.New("connection closed")

// MessageType identifies the kind of message delivered on a subscription channel.
type MessageType int

const (
	MessageTypeUnknown         MessageType = iota
	MessageTypeData                        // normal data payload
	MessageTypeError                       // GraphQL-level error from server (has Payload)
	MessageTypeComplete                    // subscription completed normally
	MessageTypeConnectionError             // connection-level error (has Err)
)

// IsTerminal reports whether the message type signals end-of-stream.
func (t MessageType) IsTerminal() bool {
	return t == MessageTypeError || t == MessageTypeComplete || t == MessageTypeConnectionError
}

// Message is a single subscription event delivered to a Handler.
type Message struct {
	Type    MessageType
	Payload *ExecutionResult
	Err     error // only set when Type == MessageTypeConnectionError
}

// Handler receives subscription messages. It is called synchronously on the
// transport's read goroutine; a slow handler blocks message delivery.
type Handler func(msg *Message)

// ExecutionResult is the GraphQL response payload for data and error messages.
type ExecutionResult struct {
	Data       json.RawMessage `json:"data,omitempty"`
	Errors     json.RawMessage `json:"errors,omitempty"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

// Request is a GraphQL operation sent to the server when subscribing.
type Request struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}
