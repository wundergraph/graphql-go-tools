package client

import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"

// Re-export common types for single-import convenience.
type (
	Message           = common.Message
	Response          = common.ExecutionResult
	GraphQLError      = common.GraphQLError
	Location          = common.Location
	Request           = common.Request
	SubscriptionError = common.SubscriptionError
	Options           = common.Options
	TransportType     = common.TransportType
	WSSubprotocol     = common.WSSubprotocol
	SSEMethod         = common.SSEMethod
)

// Re-export constants.
const (
	TransportWS  = common.TransportWS
	TransportSSE = common.TransportSSE

	SubprotocolAuto       = common.SubprotocolAuto
	SubprotocolGraphQLTWS = common.SubprotocolGraphQLTWS
	SubprotocolGraphQLWS  = common.SubprotocolGraphQLWS

	SSEMethodAuto = common.SSEMethodAuto
	SSEMethodPOST = common.SSEMethodPOST
	SSEMethodGET  = common.SSEMethodGET
)
