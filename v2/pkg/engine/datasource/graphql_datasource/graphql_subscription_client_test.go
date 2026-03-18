package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	client "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type testBridgeUpdater struct {
	updates   [][]byte
	errors    [][]byte
	completed bool
	done      bool
}

func (t *testBridgeUpdater) Update(data []byte) {
	t.updates = append(t.updates, data)
}

func (t *testBridgeUpdater) UpdateSubscription(id resolve.SubscriptionIdentifier, data []byte) {}

func (t *testBridgeUpdater) Complete() {
	t.completed = true
}

func (t *testBridgeUpdater) Error(data []byte) {
	t.errors = append(t.errors, data)
}

func (t *testBridgeUpdater) Done() {
	t.done = true
}

func (t *testBridgeUpdater) CloseSubscription(id resolve.SubscriptionIdentifier) {
}

func (t *testBridgeUpdater) Subscriptions() map[context.Context]resolve.SubscriptionIdentifier {
	return map[context.Context]resolve.SubscriptionIdentifier{}
}

func TestHandlerDeliversCorrectMessageForEachType(t *testing.T) {
	buildHandler := func(updater *testBridgeUpdater) client.Handler {
		return func(msg *client.Message) {
			switch msg.Type {
			case client.MessageTypeConnectionError:
				updater.Error(formatUpstreamServiceError(msg.Err))
				updater.Done()
			case client.MessageTypeError:
				data, _ := json.Marshal(msg.Payload)
				updater.Error(data)
				updater.Done()
			case client.MessageTypeData:
				data, err := json.Marshal(msg.Payload)
				if err != nil {
					updater.Error(formatSubscriptionError(err))
					updater.Done()
					return
				}
				updater.Update(data)
			case client.MessageTypeComplete:
				updater.Complete()
				updater.Done()
			}
		}
	}

	t.Run("connection errors deliver error and done without updates", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildHandler(updater)

		handler(&client.Message{Type: client.MessageTypeConnectionError, Err: client.ErrConnectionClosed})

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		require.Len(t, updater.updates, 0)
		require.False(t, updater.completed)
	})

	t.Run("non-connection errors deliver error and done without updates", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildHandler(updater)

		handler(&client.Message{Type: client.MessageTypeConnectionError, Err: errors.New("validation failed")})

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		require.Len(t, updater.updates, 0)
		require.False(t, updater.completed)
	})

	t.Run("complete message calls complete then done", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildHandler(updater)

		handler(&client.Message{Type: client.MessageTypeComplete})

		require.True(t, updater.done)
		require.True(t, updater.completed)
		require.Len(t, updater.errors, 0)
	})
}
