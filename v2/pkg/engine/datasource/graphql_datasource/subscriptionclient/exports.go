package client

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport"
)

// Re-export common types for single-import convenience.

type (
	Message         = common.Message
	MessageType     = common.MessageType
	ExecutionResult = common.ExecutionResult
	Request         = common.Request
	Options         = common.Options
	Handler         = common.Handler
	TransportType   = common.TransportType
	WSSubprotocol   = common.WSSubprotocol
	SSEMethod       = common.SSEMethod
)

// Re-export constants.

const (
	MessageTypeUnknown         = common.MessageTypeUnknown
	MessageTypeData            = common.MessageTypeData
	MessageTypeError           = common.MessageTypeError
	MessageTypeComplete        = common.MessageTypeComplete
	MessageTypeConnectionError = common.MessageTypeConnectionError

	TransportWS  = common.TransportWS
	TransportSSE = common.TransportSSE

	SubprotocolAuto               = common.SubprotocolAuto
	SubprotocolGraphQLTransportWS = common.SubprotocolGraphQLTransportWS
	SubprotocolGraphQLWS          = common.SubprotocolGraphQLWS

	SSEMethodAuto = common.SSEMethodAuto
	SSEMethodPOST = common.SSEMethodPOST
	SSEMethodGET  = common.SSEMethodGET
)

// Re-export error types.

type (
	ErrFailedUpgrade      = transport.ErrFailedUpgrade
	ErrInvalidSubprotocol = transport.ErrInvalidSubprotocol
)

// Re-export sentinel errors.

var (
	ErrConnectionClosed   = common.ErrConnectionClosed
	ErrConnectionError    = protocol.ErrConnectionError
	ErrAckTimeout         = protocol.ErrAckTimeout
	ErrAckNotReceived     = protocol.ErrAckNotReceived
	ErrSubscriptionExists = transport.ErrSubscriptionExists
	ErrDialFailed         = transport.ErrDialFailed
	ErrInitFailed         = transport.ErrInitFailed
)
