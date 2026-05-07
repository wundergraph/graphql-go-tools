package graph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualSubscriptionEventSource_RegistersIndependentEmitHandles(t *testing.T) {
	source := NewManualSubscriptionEventSource()

	first := source.NewSubscription()
	second := source.NewSubscription()

	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.NotSame(t, first, second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	registeredFirst, err := source.NextSubscription(ctx)
	require.NoError(t, err)
	registeredSecond, err := source.NextSubscription(ctx)
	require.NoError(t, err)

	assert.Same(t, first, registeredFirst)
	assert.Same(t, second, registeredSecond)

	select {
	case <-first.Events():
		t.Fatal("first subscription emitted before explicit trigger")
	default:
	}

	select {
	case <-second.Events():
		t.Fatal("second subscription emitted before explicit trigger")
	default:
	}

	first.Emit()
	second.Emit()

	select {
	case <-first.Events():
	case <-time.After(time.Second):
		t.Fatal("expected first subscription event after explicit trigger")
	}

	select {
	case <-second.Events():
	case <-time.After(time.Second):
		t.Fatal("expected second subscription event after explicit trigger")
	}
}
