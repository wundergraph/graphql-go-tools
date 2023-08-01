package graphql_datasource

// common
var (
	connectionInitMessage = []byte(`{"type":"connection_init"}`)
)

const (
	messageTypeConnectionAck = "connection_ack"
	messageTypeComplete      = "complete"
	messageTypeError         = "error"
)

// websocket sub-protocol:
// https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
const (
	ProtocolGraphQLWS = "graphql-ws"

	startMessage = `{"type":"start","id":"%s","payload":%s}`
	stopMessage  = `{"type":"stop","id":"%s"}`

	messageTypeConnectionKeepAlive = "ka"
	messageTypeData                = "data"
	messageTypeConnectionError     = "connection_error"
)

// websocket sub-protocol:
// https://github.com/enisdenjo/graphql-ws/blob/master/PROTOCOL.md
const (
	ProtocolGraphQLTWS = "graphql-transport-ws"

	subscribeMessage = `{"id":"%s","type":"subscribe","payload":%s}`
	pongMessage      = `{"type":"pong"}`
	completeMessage  = `{"id":"%s","type":"complete"}`

	messageTypePing = "ping"
	messageTypeNext = "next"
)

// internal
const (
	internalError        = `{"errors":[{"message":"internal error"}]}`
	connectionError      = `{"errors":[{"message":"connection error"}]}`
	errorMessageTemplate = `{"errors":[{"message":"%s"}]}`
)
