package subscription

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInitialHttpRequestContext(t *testing.T) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:8080", bytes.NewBufferString("lorem ipsum"))
	require.NoError(t, err)

	initialReqCtx := NewInitialHttpRequestContext(req)
	assert.Equal(t, ctx, initialReqCtx.Context)
	assert.Equal(t, req, initialReqCtx.Request)
}

func TestSubscriptionCancellations(t *testing.T) {
	cancellations := subscriptionCancellations{}
	var ctx context.Context

	t.Run("should add a cancellation func to map", func(t *testing.T) {
		require.Equal(t, 0, len(cancellations))

		ctx = cancellations.AddWithParent("1", context.Background())
		assert.Equal(t, 1, len(cancellations))
		assert.NotNil(t, ctx)
	})

	t.Run("should execute cancellation from map", func(t *testing.T) {
		require.Equal(t, 1, len(cancellations))
		ctxTestFunc := func() bool {
			<-ctx.Done()
			return true
		}

		ok := cancellations.Cancel("1")
		assert.Eventually(t, ctxTestFunc, time.Second, 5*time.Millisecond)
		assert.True(t, ok)
		assert.Equal(t, 0, len(cancellations))
	})
}
