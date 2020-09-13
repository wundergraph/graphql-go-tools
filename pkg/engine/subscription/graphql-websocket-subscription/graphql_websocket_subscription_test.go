package graphql_websocket_subscription

import (
	"context"
	"fmt"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
)

func TestGraphQLWebsocketSubscriptionStream(t *testing.T) {
	stream := New()
	ctx,cancel := context.WithCancel(context.Background())
	go stream.Run(ctx.Done())
	manager := subscription.NewManager(stream)
	defer cancel()
	manager.Run(ctx.Done())

	trigger := manager.StartTrigger([]byte(`{"scheme":"ws","host":"localhost:4444","path":"/"}`))
	for {
		data,ok := trigger.Next(context.Background())
		if !ok {
			return
		}
		fmt.Println(string(data))
	}
}
