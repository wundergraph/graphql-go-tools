package subscription

import (
	"context"
	"net/http"
	"sync"
)

type InitialHttpRequestContext struct {
	context.Context
	Request *http.Request
}

func NewInitialHttpRequestContext(r *http.Request) *InitialHttpRequestContext {
	return &InitialHttpRequestContext{
		Context: r.Context(),
		Request: r,
	}
}

type subscriptionCancellations struct {
	mu            sync.RWMutex
	cancellations map[string]context.CancelFunc
}

func (sc *subscriptionCancellations) AddWithParent(id string, parent context.Context) context.Context {
	ctx, cancelFunc := context.WithCancel(parent)
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancellations == nil {
		sc.cancellations = make(map[string]context.CancelFunc)
	}
	sc.cancellations[id] = cancelFunc
	return ctx
}

func (sc *subscriptionCancellations) Cancel(id string) (ok bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	cancelFunc, ok := sc.cancellations[id]
	if !ok {
		return false
	}

	cancelFunc()
	delete(sc.cancellations, id)
	return true
}

func (sc *subscriptionCancellations) CancelAll() {
	// We have full control over the cancellation functions (see AddWithParent()), so
	// it's fine to invoke them with the lock held
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	for _, cancelFunc := range sc.cancellations {
		cancelFunc()
	}
}

func (sc *subscriptionCancellations) Len() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.cancellations)
}
