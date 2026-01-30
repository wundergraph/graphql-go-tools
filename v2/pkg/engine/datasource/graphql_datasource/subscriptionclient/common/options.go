package common

import (
	"net/http"
)

type TransportType string

const (
	TransportWS  TransportType = "ws"
	TransportSSE TransportType = "sse"
)

type WSSubprotocol string

const (
	SubprotocolAuto       WSSubprotocol = ""                     // Auto, negotiated with the server
	SubprotocolGraphQLTWS WSSubprotocol = "graphql-transport-ws" // Modern subprotocol, confusingly named `graphql-ws`
	SubprotocolGraphQLWS  WSSubprotocol = "graphql-ws"           // Legacy subprotocol, deprecated
)

type SSEMethod string

const (
	SSEMethodAuto SSEMethod = ""     // Auto: POST for graphql-sse (default)
	SSEMethodPOST SSEMethod = "POST" // POST with JSON body (graphql-sse spec)
	SSEMethodGET  SSEMethod = "GET"  // GET with query parameters (traditional SSE)
)

type Options struct {
	Endpoint    string
	Headers     http.Header
	InitPayload map[string]any
	Transport   TransportType

	// Only affects the WebSocket transport.
	WSSubprotocol WSSubprotocol

	// Only affects the SSE transport.
	// Defaults to POST (graphql-sse spec). Use GET for traditional SSE endpoints.
	SSEMethod SSEMethod
}
