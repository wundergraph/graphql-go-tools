package subscription

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
)

var (
	ErrSubscriberIDAlreadyExists = errors.New("subscriber id already exists")
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

func (sc *subscriptionCancellations) AddWithParent(id string, parent context.Context) (context.Context, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancellations == nil {
		sc.cancellations = make(map[string]context.CancelFunc)
	}
	if _, ok := sc.cancellations[id]; ok {
		return nil, fmt.Errorf("%w: %s", ErrSubscriberIDAlreadyExists, id)
	}
	ctx, cancelFunc := context.WithCancel(parent)
	sc.cancellations[id] = cancelFunc
	return ctx, nil
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
