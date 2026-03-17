package graphql_datasource

import (
	"context"
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

func TestReadLoopErrorHandling(t *testing.T) {
	t.Run("connection errors deliver error and done without updates", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *client.Message, 1)
		msgCh <- &client.Message{Err: client.ErrConnectionClosed}
		close(msgCh)

		subClient := &subscriptionClientV2{}
		subClient.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		require.Len(t, updater.updates, 0)
		require.False(t, updater.completed)
	})

	t.Run("non-connection errors deliver error and done without updates", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *client.Message, 1)
		msgCh <- &client.Message{Err: errors.New("validation failed")}
		close(msgCh)

		subClient := &subscriptionClientV2{}
		subClient.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.done)
		require.Len(t, updater.errors, 1)
		require.Len(t, updater.updates, 0)
		require.False(t, updater.completed)
	})

	t.Run("context cancellation calls done without complete", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msgCh := make(chan *client.Message)

		subClient := &subscriptionClientV2{}
		subClient.readLoop(ctx, msgCh, func() {}, updater)

		require.True(t, updater.done)
		require.False(t, updater.completed)
	})

	t.Run("channel close calls done without complete", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *client.Message)
		close(msgCh)

		subClient := &subscriptionClientV2{}
		subClient.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.done)
		require.False(t, updater.completed)
	})

	t.Run("done message calls complete then done", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *client.Message, 1)
		msgCh <- &client.Message{Done: true}
		close(msgCh)

		subClient := &subscriptionClientV2{}
		subClient.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.done)
		require.True(t, updater.completed)
		require.Len(t, updater.errors, 0)
	})
}
