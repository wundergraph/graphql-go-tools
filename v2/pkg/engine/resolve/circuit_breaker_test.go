package resolve

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingCache is a test LoaderCache that fails on demand.
// Uses atomic counters for goroutine safety in concurrent tests.
type failingCache struct {
	getErr    error
	setErr    error
	deleteErr error
	getCalls  atomic.Int64
	setCalls  atomic.Int64
	delCalls  atomic.Int64
}

func (c *failingCache) Get(_ context.Context, _ []string) ([]*CacheEntry, error) {
	c.getCalls.Add(1)
	if c.getErr != nil {
		return nil, c.getErr
	}
	return []*CacheEntry{{Key: "k", Value: []byte("v")}}, nil
}

func (c *failingCache) Set(_ context.Context, _ []*CacheEntry, _ time.Duration) error {
	c.setCalls.Add(1)
	return c.setErr
}

func (c *failingCache) Delete(_ context.Context, _ []string) error {
	c.delCalls.Add(1)
	return c.deleteErr
}

func TestCircuitBreaker(t *testing.T) {
	cacheErr := errors.New("redis: connection refused")

	t.Run("closed - passes through on success", func(t *testing.T) {
		inner := &failingCache{}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 3,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		entries, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, int64(1), inner.getCalls.Load())

		err = cb.Set(ctx, []*CacheEntry{{Key: "k1"}}, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, int64(1), inner.setCalls.Load())

		err = cb.Delete(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), inner.delCalls.Load())
	})

	t.Run("stays closed below threshold", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 3,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		// Two failures — below threshold of 3
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})

		assert.Equal(t, int64(2), inner.getCalls.Load(), "both calls should pass through")
		assert.False(t, cb.state.isOpen(), "breaker should remain closed")

		// Third call still passes through (threshold is reached ON this call)
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.Equal(t, int64(3), inner.getCalls.Load(), "threshold call should pass through")
		assert.True(t, cb.state.isOpen(), "breaker should be open after reaching threshold")
	})

	t.Run("opens after consecutive failures reach threshold", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 2,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.True(t, cb.state.isOpen())

		// While open, Get returns nil/nil, inner is not called
		entries, err := cb.Get(ctx, []string{"k1"})
		assert.NoError(t, err, "open breaker returns nil error")
		assert.Nil(t, entries, "open breaker returns nil entries (all-miss)")
		assert.Equal(t, int64(2), inner.getCalls.Load(), "inner should not be called when open")
	})

	t.Run("open breaker skips Set and Delete", func(t *testing.T) {
		inner := &failingCache{setErr: cacheErr, deleteErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   time.Second,
		})
		// Force open
		state.openedAt.Store(time.Now().UnixNano())

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		err := cb.Set(ctx, []*CacheEntry{{Key: "k1"}}, time.Minute)
		assert.NoError(t, err, "open breaker Set returns nil")
		assert.Equal(t, int64(0), inner.setCalls.Load(), "inner Set not called when open")

		err = cb.Delete(ctx, []string{"k1"})
		assert.NoError(t, err, "open breaker Delete returns nil")
		assert.Equal(t, int64(0), inner.delCalls.Load(), "inner Delete not called when open")
	})

	t.Run("half-open probe success closes breaker", func(t *testing.T) {
		inner := &failingCache{} // no errors — probe succeeds
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 2,
			CooldownPeriod:   10 * time.Millisecond,
		})
		// Open the breaker in the past so cooldown has elapsed
		state.openedAt.Store(time.Now().Add(-50 * time.Millisecond).UnixNano())
		state.consecutiveFailures.Store(2)

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		entries, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Len(t, entries, 1, "probe should return data")
		assert.Equal(t, int64(1), inner.getCalls.Load(), "probe should call inner")
		assert.False(t, cb.state.isOpen(), "breaker should be closed after successful probe")
		assert.Equal(t, int64(0), cb.state.consecutiveFailures.Load(), "failures should be reset")
	})

	t.Run("half-open probe failure re-opens breaker", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   10 * time.Millisecond,
		})
		// Open the breaker in the past so cooldown has elapsed
		state.openedAt.Store(time.Now().Add(-50 * time.Millisecond).UnixNano())

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		_, err := cb.Get(ctx, []string{"k1"})
		assert.Error(t, err, "probe failure should return error")
		assert.Equal(t, int64(1), inner.getCalls.Load(), "probe should call inner")
		assert.True(t, cb.state.isOpen(), "breaker should re-open after failed probe")
	})

	t.Run("success resets consecutive failure count", func(t *testing.T) {
		inner := &failingCache{}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 3,
			CooldownPeriod:   time.Second,
		})

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()

		// Two failures
		inner.getErr = cacheErr
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.Equal(t, int64(2), state.consecutiveFailures.Load())

		// One success resets count
		inner.getErr = nil
		_, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Equal(t, int64(0), state.consecutiveFailures.Load(), "success should reset failures")
		assert.False(t, state.isOpen())
	})

	t.Run("concurrent access safety", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 100, // high threshold so we can count
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				_, _ = cb.Get(ctx, []string{"k1"})
			})
		}
		wg.Wait()

		// No panics, no data races. Exact failure count may vary due to
		// concurrency but should be <= 50.
		assert.LessOrEqual(t, cb.state.consecutiveFailures.Load(), int64(50))
	})

	t.Run("wrapCachesWithCircuitBreakers applies defaults", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: true}, // no threshold or cooldown set
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		wrapped, ok := result["default"].(*circuitBreakerCache)
		require.True(t, ok, "cache should be wrapped")
		assert.Equal(t, 5, wrapped.state.config.FailureThreshold, "default threshold should be 5")
		assert.Equal(t, 10*time.Second, wrapped.state.config.CooldownPeriod, "default cooldown should be 10s")
		// Original map should not be mutated
		_, originalWrapped := caches["default"].(*circuitBreakerCache)
		assert.False(t, originalWrapped, "original map should not be mutated")
	})

	t.Run("wrapCachesWithCircuitBreakers skips disabled", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: false},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok, "disabled breaker should not wrap the cache")
	})

	t.Run("wrapCachesWithCircuitBreakers ignores missing cache names", func(t *testing.T) {
		caches := map[string]LoaderCache{"default": &failingCache{}}
		configs := map[string]CircuitBreakerConfig{
			"nonexistent": {Enabled: true},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok, "unrelated cache should not be wrapped")
	})
}
