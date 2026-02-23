package graphql_datasource

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/protocol"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type testBridgeUpdater struct {
	updates   [][]byte
	closed    bool
	closeKind resolve.SubscriptionCloseKind
	completed bool
}

func (t *testBridgeUpdater) Update(data []byte) {
	t.updates = append(t.updates, data)
}

func (t *testBridgeUpdater) UpdateSubscription(id resolve.SubscriptionIdentifier, data []byte) {}

func (t *testBridgeUpdater) Complete() {
	t.completed = true
}

func (t *testBridgeUpdater) Close(kind resolve.SubscriptionCloseKind) {
	t.closed = true
	t.closeKind = kind
}

func (t *testBridgeUpdater) CloseSubscription(kind resolve.SubscriptionCloseKind, id resolve.SubscriptionIdentifier) {
}

func (t *testBridgeUpdater) Subscriptions() map[context.Context]resolve.SubscriptionIdentifier {
	return map[context.Context]resolve.SubscriptionIdentifier{}
}

func TestCloseKindForMessageError(t *testing.T) {
	t.Run("connection closed uses downstream service error close kind", func(t *testing.T) {
		closeKind, sendPayload := closeKindForMessageError(common.ErrConnectionClosed)
		require.Equal(t, resolve.SubscriptionCloseKindDownstreamServiceError, closeKind)
		require.False(t, sendPayload)
	})

	t.Run("connection error uses downstream service error close kind", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", protocol.ErrConnectionError)
		closeKind, sendPayload := closeKindForMessageError(err)
		require.Equal(t, resolve.SubscriptionCloseKindDownstreamServiceError, closeKind)
		require.False(t, sendPayload)
	})

	t.Run("generic errors use normal close kind and payload", func(t *testing.T) {
		closeKind, sendPayload := closeKindForMessageError(errors.New("boom"))
		require.Equal(t, resolve.SubscriptionCloseKindNormal, closeKind)
		require.True(t, sendPayload)
	})
}

func TestSubscriptionClientV2ReadLoopCloseKinds(t *testing.T) {
	t.Run("connection errors close as downstream service error without payload", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *common.Message, 1)
		msgCh <- &common.Message{Err: common.ErrConnectionClosed}
		close(msgCh)

		client := &subscriptionClientV2{}
		client.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.closed)
		require.Equal(t, resolve.SubscriptionCloseKindDownstreamServiceError, updater.closeKind)
		require.Len(t, updater.updates, 0)
		require.False(t, updater.completed)
	})

	t.Run("non-connection errors send payload and close normally", func(t *testing.T) {
		updater := &testBridgeUpdater{}
		msgCh := make(chan *common.Message, 1)
		msgCh <- &common.Message{Err: errors.New("validation failed")}
		close(msgCh)

		client := &subscriptionClientV2{}
		client.readLoop(context.Background(), msgCh, func() {}, updater)

		require.True(t, updater.closed)
		require.Equal(t, resolve.SubscriptionCloseKindNormal, updater.closeKind)
		require.Len(t, updater.updates, 1)
		require.False(t, updater.completed)
	})
}
