package transport_test

import (
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

func receiveWithTimeout(t *testing.T, ch <-chan *common.Message, timeout time.Duration) *common.Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatal("timeout waiting for message")
		return nil
	}
}
