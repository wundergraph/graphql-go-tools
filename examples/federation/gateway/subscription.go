package main

import (
	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
	graphql_websocket_subscription "github.com/jensneuse/graphql-go-tools/pkg/engine/subscription/graphql-websocket-subscription"
)

func newSubscriptionManager() *subscription.Manager {
	stream := graphql_websocket_subscription.New()
	return subscription.NewManager(stream)
}
