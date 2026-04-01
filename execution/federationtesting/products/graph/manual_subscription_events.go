package graph

import (
	"context"
)

// ManualSubscriptionEventSource registers one explicit emit handle per active
// subscription so tests can control event delivery deterministically.
type ManualSubscriptionEventSource struct {
	registered chan *ManualSubscriptionHandle
}

// ManualSubscriptionHandle is the per-subscription trigger used by tests.
type ManualSubscriptionHandle struct {
	events chan struct{}
}

func NewManualSubscriptionEventSource() *ManualSubscriptionEventSource {
	return &ManualSubscriptionEventSource{
		registered: make(chan *ManualSubscriptionHandle, 64),
	}
}

func (s *ManualSubscriptionEventSource) NewSubscription() *ManualSubscriptionHandle {
	handle := &ManualSubscriptionHandle{
		events: make(chan struct{}, 16),
	}
	s.registered <- handle
	return handle
}

func (s *ManualSubscriptionEventSource) NextSubscription(ctx context.Context) (*ManualSubscriptionHandle, error) {
	select {
	case handle := <-s.registered:
		return handle, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (h *ManualSubscriptionHandle) Emit() {
	h.events <- struct{}{}
}

func (h *ManualSubscriptionHandle) Events() <-chan struct{} {
	return h.events
}
