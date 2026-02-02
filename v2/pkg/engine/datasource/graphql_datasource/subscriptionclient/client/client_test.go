package client

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionKey(t *testing.T) {
	t.Run("differs for different queries", func(t *testing.T) {
		opts := Options{Endpoint: "ws://localhost/graphql"}

		key1 := subscriptionKey(opts, &Request{Query: "subscription { a }"})
		key2 := subscriptionKey(opts, &Request{Query: "subscription { b }"})

		assert.NotEqual(t, key1, key2)
	})

	t.Run("is deterministic", func(t *testing.T) {
		opts := Options{Endpoint: "ws://localhost/graphql"}
		req := &Request{Query: "subscription { a }"}

		key1 := subscriptionKey(opts, req)
		key2 := subscriptionKey(opts, req)

		assert.Equal(t, key1, key2)
	})

	t.Run("differs for different variables", func(t *testing.T) {
		opts := Options{Endpoint: "ws://localhost/graphql"}
		query := "subscription($id: ID!) { item(id: $id) }"

		key1 := subscriptionKey(opts, &Request{Query: query, Variables: map[string]any{"id": "1"}})
		key2 := subscriptionKey(opts, &Request{Query: query, Variables: map[string]any{"id": "2"}})

		assert.NotEqual(t, key1, key2)
	})

	t.Run("differs for different endpoints", func(t *testing.T) {
		req := &Request{Query: "subscription { a }"}

		key1 := subscriptionKey(Options{Endpoint: "ws://a.example.com/graphql"}, req)
		key2 := subscriptionKey(Options{Endpoint: "ws://b.example.com/graphql"}, req)

		assert.NotEqual(t, key1, key2)
	})

	t.Run("differs for different extensions", func(t *testing.T) {
		opts := Options{Endpoint: "ws://localhost/graphql"}
		query := "subscription { a }"

		key1 := subscriptionKey(opts, &Request{Query: query, Extensions: map[string]any{"subId": 1}})
		key2 := subscriptionKey(opts, &Request{Query: query, Extensions: map[string]any{"subId": 2}})

		assert.NotEqual(t, key1, key2)
	})
}

func TestSubscription(t *testing.T) {
	t.Run("addListener creates buffered channel", func(t *testing.T) {
		sub := &subscription{
			listeners: make(map[uint64]chan *Message),
		}

		ch, _, err := sub.addListener()
		require.NoError(t, err)

		assert.NotNil(t, ch)
		assert.Equal(t, 8, cap(ch))
	})

	t.Run("multiple listeners get unique IDs", func(t *testing.T) {
		sub := &subscription{
			listeners: make(map[uint64]chan *Message),
		}

		sub.addListener()
		sub.addListener()
		sub.addListener()

		assert.Equal(t, 3, len(sub.listeners))
	})

	t.Run("removeListener removes from map", func(t *testing.T) {
		sub := &subscription{
			cancelFn:  func() {},
			listeners: make(map[uint64]chan *Message),
		}

		_, cancel, _ := sub.addListener()
		assert.Equal(t, 1, len(sub.listeners))

		cancel()
		assert.Equal(t, 0, len(sub.listeners))
	})

	t.Run("last listener removal cancels upstream", func(t *testing.T) {
		var cancelled atomic.Bool

		sub := &subscription{
			cancelFn:  func() { cancelled.Store(true) },
			listeners: make(map[uint64]chan *Message),
		}

		_, cancel1, _ := sub.addListener()
		_, cancel2, _ := sub.addListener()

		cancel1()
		assert.False(t, cancelled.Load(), "should not cancel with listeners remaining")

		cancel2()
		assert.True(t, cancelled.Load(), "should cancel when last listener removed")
	})

	t.Run("fanout broadcasts to all listeners", func(t *testing.T) {
		source := make(chan *Message, 1)
		sub := &subscription{
			source:    source,
			cancelFn:  func() {},
			listeners: make(map[uint64]chan *Message),
			done:      make(chan struct{}),
		}

		ch1, _, _ := sub.addListener()
		ch2, _, _ := sub.addListener()

		go sub.fanout(func() {})

		msg := &Message{Payload: &ExecutionResult{}}
		source <- msg
		close(source)

		// Wait for fanout to complete
		<-sub.done

		assert.Equal(t, msg, <-ch1)
		assert.Equal(t, msg, <-ch2)
	})

	t.Run("fanout closes listeners when source closes", func(t *testing.T) {
		source := make(chan *Message)
		sub := &subscription{
			source:    source,
			cancelFn:  func() {},
			listeners: make(map[uint64]chan *Message),
			done:      make(chan struct{}),
		}

		ch, _, _ := sub.addListener()

		go sub.fanout(func() {})

		close(source)
		<-sub.done

		_, ok := <-ch
		assert.False(t, ok, "channel should be closed")
	})
}

func TestClient(t *testing.T) {
	t.Run("New creates client with transports", func(t *testing.T) {
		c := New(t.Context(), http.DefaultClient, http.DefaultClient)

		assert.NotNil(t, c.ws)
		assert.NotNil(t, c.sse)
		assert.NotNil(t, c.subs)
	})

	t.Run("Stats returns correct counts", func(t *testing.T) {
		c := New(t.Context(), http.DefaultClient, http.DefaultClient)

		stats := c.Stats()
		assert.Equal(t, 0, stats.Subscriptions)
		assert.Equal(t, 0, stats.Listeners)
	})

	t.Run("context cancellation is idempotent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		_ = New(ctx, http.DefaultClient, http.DefaultClient)
		cancel()
		cancel() // should not panic
	})

	t.Run("Subscribe fails after context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := New(ctx, http.DefaultClient, http.DefaultClient)
		cancel()

		_, _, err := c.Subscribe(t.Context(), &Request{Query: "subscription { a }"}, Options{
			Endpoint: "ws://localhost/graphql",
		})
		assert.Equal(t, ErrClientClosed, err)
	})
}

func TestClientDedup(t *testing.T) {
	t.Run("identical subscriptions share upstream", func(t *testing.T) {
		opts := Options{Endpoint: "ws://localhost/graphql"}
		req := &Request{Query: "subscription { a }"}

		key1 := subscriptionKey(opts, req)
		key2 := subscriptionKey(opts, req)

		assert.Equal(t, key1, key2, "identical requests should produce same key")
	})
}

func TestConcurrency(t *testing.T) {
	t.Run("subscription handles concurrent listener add/remove", func(t *testing.T) {
		source := make(chan *Message)
		sub := &subscription{
			source:    source,
			cancelFn:  func() { close(source) },
			listeners: make(map[uint64]chan *Message),
			done:      make(chan struct{}),
		}

		go sub.fanout(func() {})

		// Keep one listener alive to prevent subscription from closing
		_, anchorCancel, err := sub.addListener()
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				_, cancel, err := sub.addListener()
				if err == nil {
					cancel()
				}
			})
		}

		wg.Wait()

		// Now release the anchor - this triggers subscription close
		anchorCancel()
		<-sub.done
	})

	t.Run("addListener returns error after subscription closed", func(t *testing.T) {
		source := make(chan *Message)
		sub := &subscription{
			source:    source,
			cancelFn:  func() { close(source) },
			listeners: make(map[uint64]chan *Message),
			done:      make(chan struct{}),
		}

		go sub.fanout(func() {})

		// Add and remove listener to close subscription
		_, cancel, _ := sub.addListener()
		cancel()
		<-sub.done

		// Now try to add another listener
		_, _, err := sub.addListener()
		assert.Equal(t, ErrSubscriptionClosed, err)
	})
}
