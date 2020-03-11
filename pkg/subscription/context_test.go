package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionCancellations(t *testing.T) {
	cancellations := subscriptionCancellations{}
	var ctx context.Context

	t.Run("should add a cancellation func to map", func(t *testing.T) {
		require.Equal(t, 0, len(cancellations))

		ctx = cancellations.Add("1")
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
