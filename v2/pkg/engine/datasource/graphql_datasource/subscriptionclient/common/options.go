package common

import (
	"net/http"
)

// TransportType selects the subscription transport mechanism.
type TransportType string

const (
	TransportWS  TransportType = "ws"  // WebSocket connection
	TransportSSE TransportType = "sse" // Server-Sent Events over HTTP
)

// WSSubprotocol selects the GraphQL-over-WebSocket subprotocol.
type WSSubprotocol string

const (
	SubprotocolAuto               WSSubprotocol = ""                     // Auto, negotiated with the server
	SubprotocolGraphQLTransportWS WSSubprotocol = "graphql-transport-ws" // Modern protocol from The Guild
	SubprotocolGraphQLWS          WSSubprotocol = "graphql-ws"           // Legacy Apollo protocol, deprecated
)

// Subprotocols returns the WebSocket subprotocol strings to offer during the upgrade handshake.
func (s WSSubprotocol) Subprotocols() []string {
	switch s {
	case SubprotocolAuto:
		return []string{"graphql-transport-ws", "graphql-ws"}
	case SubprotocolGraphQLTransportWS:
		return []string{"graphql-transport-ws"}
	case SubprotocolGraphQLWS:
		return []string{"graphql-ws"}
	default:
		return nil
	}
}

type SSEMethod string

const (
	SSEMethodAuto SSEMethod = ""     // Auto: POST for graphql-sse (default)
	SSEMethodPOST SSEMethod = "POST" // POST with JSON body (graphql-sse spec)
	SSEMethodGET  SSEMethod = "GET"  // GET with query parameters (traditional SSE)
)

// Options configures a single subscription request (endpoint, headers, transport selection).
type Options struct {
	Endpoint    string
	Headers     http.Header
	InitPayload map[string]any
	Transport   TransportType

	// Only affects the WebSocket transport.
	WSSubprotocol WSSubprotocol

	// Only affects the SSE transport.
	// Defaults to POST (graphql-sse spec).
	SSEMethod SSEMethod
}
