package graphql

type SubscriptionType int

const (
	// SubscriptionTypeUnknown is for unknown or undefined subscriptions.
	SubscriptionTypeUnknown = iota
	// SubscriptionTypeSSE is for Server-Sent Events (SSE) subscriptions.
	SubscriptionTypeSSE
	// SubscriptionTypeGraphQLWS is for subscriptions using a WebSocket connection with
	// 'graphql-ws' as protocol.
	SubscriptionTypeGraphQLWS
	// SubscriptionTypeGraphQLTransportWS is for subscriptions using a WebSocket connection with
	// 'graphql-transport-ws' as protocol.
	SubscriptionTypeGraphQLTransportWS
)
