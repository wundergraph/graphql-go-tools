package graphql_datasource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestBuildMessageHandlerRoutesEachMessageTypeCorrectly(t *testing.T) {
	t.Run("error is upstream service error for connection error", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildMessageHandler(updater, "DOWNSTREAM_SERVICE_ERROR")

		handler(&client.Message{Type: client.MessageTypeConnectionError, Err: client.ErrConnectionClosed})

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		assert.Contains(t, string(updater.errors[0]), "DOWNSTREAM_SERVICE_ERROR")
		require.Empty(t, updater.updates)
		require.False(t, updater.completed)
	})

	t.Run("error contains payload for graphql error", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildMessageHandler(updater, "DOWNSTREAM_SERVICE_ERROR")

		handler(&client.Message{
			Type: client.MessageTypeError,
			Payload: &client.ExecutionResult{
				Errors: json.RawMessage(`[{"message":"field not found"}]`),
			},
		})

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		assert.Contains(t, string(updater.errors[0]), "field not found")
		require.Empty(t, updater.updates)
		require.False(t, updater.completed)
	})

	t.Run("update is delivered without completing for data message", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildMessageHandler(updater, "DOWNSTREAM_SERVICE_ERROR")

		handler(&client.Message{
			Type: client.MessageTypeData,
			Payload: &client.ExecutionResult{
				Data: json.RawMessage(`{"foo":"bar"}`),
			},
		})

		require.Len(t, updater.updates, 1)
		assert.JSONEq(t, `{"data":{"foo":"bar"}}`, string(updater.updates[0]))
		require.False(t, updater.done)
		require.False(t, updater.completed)
		require.Empty(t, updater.errors)
	})

	t.Run("complete and done are set for complete message", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		handler := buildMessageHandler(updater, "DOWNSTREAM_SERVICE_ERROR")

		handler(&client.Message{Type: client.MessageTypeComplete})

		require.True(t, updater.done)
		require.True(t, updater.completed)
		require.Empty(t, updater.errors)
	})
}
