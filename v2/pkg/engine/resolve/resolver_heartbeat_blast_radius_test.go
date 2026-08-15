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

// TestResolver_StuckWriteBlocksHeartbeatForUnrelatedTrigger reproduces, at the
// unit level, the mechanism found live in a production pprof capture: one
// subscriber's writeMu, held for the duration of a stuck downstream write,
// blocks the pod-wide heartbeat sweep from ever reaching a completely
// unrelated subscriber on a different trigger — because heartbeatLoop /
// sendTriggerHeartbeats process every trigger sequentially in one goroutine,
// not concurrently.
//
// This does not use a real network write. subA.writeMu is locked directly by
// the test to stand in for a real stuck sub.writer.Flush() (resolve.go:1077,
// inside executeSubscriptionUpdate) that has acquired writeMu but not yet
// released it — mechanically identical from the heartbeat sweep's point of
// view, since it only ever contends on the mutex, never on the write itself.
func TestResolver_StuckWriteBlocksHeartbeatForUnrelatedTrigger(t *testing.T) {
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
	subA.writeMu.Lock()

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

	// While subA's write stays stuck, trigger 2's heartbeat must NOT fire —
	// even though subB's own writeMu was never touched.
	select {
	case <-heartbeatReceived:
		t.Fatal("healthy trigger's heartbeat fired while the unrelated trigger's write was still stuck — bug not reproduced")
	case <-time.After(200 * time.Millisecond):
		// expected: still blocked behind the stuck trigger
	}

	// Release the stuck write, exactly as a dead connection eventually
	// getting torn down would release it in production.
	subA.writeMu.Unlock()

	select {
	case <-heartbeatReceived:
		// expected: now that the stuck trigger's sweep can complete, the
		// healthy trigger's heartbeat goes through promptly.
	case <-time.After(time.Second):
		t.Fatal("healthy trigger's heartbeat never fired after the stuck write was released")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat sweep never completed")
	}
}
