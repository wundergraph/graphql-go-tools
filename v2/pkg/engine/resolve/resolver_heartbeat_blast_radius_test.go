package resolve

import (
	"context"
	"sync"
	"testing"
	"time"
)

// RecordingHeartbeatWriter is a SubscriptionResponseWriter whose Heartbeat call
// is observable from the test via a callback, with no other side effects.
type RecordingHeartbeatWriter struct {
	onHeartbeat func()
}

func (r *RecordingHeartbeatWriter) Write(p []byte) (n int, err error) { return len(p), nil }
func (r *RecordingHeartbeatWriter) Flush() error                      { return nil }
func (r *RecordingHeartbeatWriter) Complete()                         {}
func (r *RecordingHeartbeatWriter) Error([]byte)                      {}
func (r *RecordingHeartbeatWriter) Heartbeat() error {
	r.onHeartbeat()
	return nil
}

var _ SubscriptionResponseWriter = (*RecordingHeartbeatWriter)(nil)

// TestResolver_StuckWriteDoesNotBlockHeartbeatForUnrelatedTrigger proves the
// fix for the blast-radius mechanism found live in a production pprof
// capture: one subscriber's writeMu, held for the duration of a stuck
// downstream write, used to block the pod-wide heartbeat sweep from ever
// reaching a completely unrelated subscriber on a different trigger, because
// heartbeatLoop / sendTriggerHeartbeats process every trigger sequentially
// in one goroutine, not concurrently. sendHeartbeat now uses writeMu.TryLock
// rather than Lock, so a stuck write simply causes that one trigger's own
// heartbeat to be skipped for this cycle instead of blocking the sweep.
//
// This does not use a real network write. subA.writeMu is locked directly by
// the test to stand in for a real stuck sub.writer.Flush() (resolve.go:1077,
// inside executeSubscriptionUpdate) that has acquired writeMu but not yet
// released it — mechanically identical from the heartbeat sweep's point of
// view, since it only ever contends on the mutex, never on the write itself.
// See TestResolver_RealDeadTCPClientBlocksHeartbeatForUnrelatedTrigger for
// the real-transport sibling proving the same fix over an actual TCP write.
func TestResolver_StuckWriteDoesNotBlockHeartbeatForUnrelatedTrigger(t *testing.T) {
	resolverCtx := t.Context()

	resolver := New(resolverCtx, ResolverOptions{
		MaxConcurrency:                1,
		AsyncErrorWriter:              &FakeErrorWriter{},
		SubscriptionHeartbeatInterval: time.Hour, // long interval so the background loop doesn't compete
	})

	const stuckTriggerID = uint64(1)
	const healthyTriggerID = uint64(2)

	subA := &subscriptionState{
		triggerID: stuckTriggerID,
		ctx:       NewContext(context.Background()),
		writer:    &FakeSubscriptionWriter{},
		id:        SubscriptionIdentifier{ConnectionID: 1, SubscriptionID: 1},
		heartbeat: true,
		completed: make(chan struct{}),
	}
	// Stand in for a real stuck sub.writer.Flush(): a data push has acquired
	// writeMu and not released it, exactly as goroutine 5563916 was captured
	// doing in production, blocked on the OS write with the lock still held.
	// Deliberately never released within this test: the fix means the
	// healthy trigger's heartbeat no longer needs to wait for it to be.
	subA.writeMu.Lock()
	defer subA.writeMu.Unlock()

	heartbeatReceived := make(chan struct{})
	var closeOnce sync.Once
	subB := &subscriptionState{
		triggerID: healthyTriggerID,
		ctx:       NewContext(context.Background()),
		writer: &RecordingHeartbeatWriter{onHeartbeat: func() {
			closeOnce.Do(func() { close(heartbeatReceived) })
		}},
		id:        SubscriptionIdentifier{ConnectionID: 2, SubscriptionID: 2},
		heartbeat: true,
		completed: make(chan struct{}),
	}

	resolver.mu.Lock()
	resolver.triggers[stuckTriggerID] = &trigger{
		id:            stuckTriggerID,
		cancel:        func() {},
		subscriptions: map[SubscriptionIdentifier]*subscriptionState{subA.id: subA},
	}
	resolver.triggers[healthyTriggerID] = &trigger{
		id:            healthyTriggerID,
		cancel:        func() {},
		subscriptions: map[SubscriptionIdentifier]*subscriptionState{subB.id: subB},
	}
	resolver.mu.Unlock()

	done := make(chan struct{})
	go func() {
		// Mirrors sendTriggerHeartbeats' own sequential loop (resolve.go:1632:
		// `for _, id := range triggerIDs { r.heartbeatTriggerSubscriptions(id) }`),
		// but with the order pinned so the test is deterministic instead of
		// depending on Go's randomized map iteration order.
		resolver.heartbeatTriggerSubscriptions(stuckTriggerID)
		resolver.heartbeatTriggerSubscriptions(healthyTriggerID)
		close(done)
	}()

	// Even with subA's write still stuck and never released, trigger 2's
	// heartbeat must fire promptly — sendHeartbeat's TryLock skips subA's
	// contended heartbeat instead of blocking the sweep behind it.
	select {
	case <-heartbeatReceived:
		// expected: the fix means this doesn't wait on the stuck trigger at all.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("healthy trigger's heartbeat did not fire promptly — stuck write is still blocking the sweep, fix not effective")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat sweep never completed")
	}
}
