package client

import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"

// Re-export common types for single-import convenience.
type (
	Message         = common.Message
	ExecutionResult = common.ExecutionResult
	Request         = common.Request
	Options         = common.Options
	TransportType   = common.TransportType
	WSSubprotocol   = common.WSSubprotocol
	SSEMethod       = common.SSEMethod
)

// Re-export constants.
const (
	TransportWS  = common.TransportWS
	TransportSSE = common.TransportSSE

	SubprotocolAuto               = common.SubprotocolAuto
	SubprotocolGraphQLTransportWS = common.SubprotocolGraphQLTransportWS
	SubprotocolGraphQLWS          = common.SubprotocolGraphQLWS

	SSEMethodAuto = common.SSEMethodAuto
	SSEMethodPOST = common.SSEMethodPOST
	SSEMethodGET  = common.SSEMethodGET
)
