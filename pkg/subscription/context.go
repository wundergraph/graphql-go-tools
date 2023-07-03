package subscription

import (
	"context"
	"fmt"
	"net/http"
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

type subscriptionCancellations map[string]context.CancelFunc

func (sc subscriptionCancellations) AddWithParent(id string, parent context.Context) (context.Context, error) {
	_, ok := sc[id]
	if ok {
		return nil, fmt.Errorf("subscriber for %s already exists", id)
	}

	ctx, cancelFunc := context.WithCancel(parent)
	sc[id] = cancelFunc
	return ctx, nil
}

func (sc subscriptionCancellations) Cancel(id string) (ok bool) {
	cancelFunc, ok := sc[id]
	if !ok {
		return false
	}

	cancelFunc()
	delete(sc, id)
	return true
}

func (sc subscriptionCancellations) CancelAll() {
	for _, cancelFunc := range sc {
		cancelFunc()
	}
}
