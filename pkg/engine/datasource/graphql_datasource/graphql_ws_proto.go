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

// https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
const (
	protocolGraphQLWS = "graphql-ws"

	startMessage    = `{"type":"start","id":"%s","payload":%s}`
	stopMessage     = `{"type":"stop","id":"%s"}`
	internalError   = `{"errors":[{"message":"connection error"}]}`
	connectionError = `{"errors":[{"message":"connection error"}]}`

	messageTypeConnectionKeepAlive = "ka"
	messageTypeData                = "data"
	messageTypeConnectionError     = "connection_error"
)

// https://github.com/enisdenjo/graphql-ws/blob/master/PROTOCOL.md
const (
	protocolGraphQL = "graphql-transport-ws"

	subscribeMessage = `{"type":"subscribe","id":"%s","payload":%s}`
	pingMessage      = `{"type":"ping"}`
	pongMessage      = `{"type":"pong"}`
	completeMessage  = `{"type":"complete","id":"%s"}`

	messageTypePing = "ping"
	messageTypePong = "pong"
	messageTypeNext = "next"
)
